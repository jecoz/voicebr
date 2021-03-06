/// Broadcast voice messages to a set of recipients.
/// Copyright (C) 2019 Daniel Morandini (jecoz)
///
/// This program is free software: you can redistribute it and/or modify
/// it under the terms of the GNU General Public License as published by
/// the Free Software Foundation, either version 3 of the License, or
/// (at your option) any later version.
///
/// This program is distributed in the hope that it will be useful,
/// but WITHOUT ANY WARRANTY; without even the implied warranty of
/// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
/// GNU General Public License for more details.
///
/// You should have received a copy of the GNU General Public License
/// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package nexmo

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
)

var (
	CallLimiter = NewLimiter(3)
	GetLimiter  = NewLimiter(15)
)

type Client struct {
	internal *http.Client
	AppID    string
	Number   string
	Origin   string
	key      interface{}
}

func NewClient(pKeyR io.Reader, appID, number, origin string) (*Client, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, pKeyR); err != nil {
		return nil, fmt.Errorf("new client error: unable to read private key: %v", err)
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("new client error: %v", err)
	}

	return &Client{
		internal: http.DefaultClient,
		AppID:    appID,
		Number:   number,
		Origin:   origin,
		key:      key,
	}, nil
}

func (c *Client) Token() (string, error) {
	if c.key == nil {
		return "", fmt.Errorf("token: found nil key. Use NewClient to create a valid Client")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"alg":            "RS256",
		"typ":            "JWT",
		"application_id": c.AppID,
		"iat":            time.Now().Unix(),
		"jti":            uuid.New().String(),
	})

	return token.SignedString(c.key)
}

func (c *Client) Get(ctx context.Context, url string) (*http.Response, error) {
	if err := GetLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("client: unable to perform Get: %v", err)
	}
	return c.Do("GET", url, nil)
}

func (c *Client) Post(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	if err := CallLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("client: unable to perform Post: %v", err)
	}
	return c.Do("POST", url, body)
}

func (c *Client) Do(method, url string, body io.Reader) (*http.Response, error) {
	token, err := c.Token()
	if err != nil {
		return nil, fmt.Errorf("unable to create authorization token: %v", err)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("unable to make request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.internal.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, checkStatus(resp)
}

type Contact struct {
	Name   string `json:"-"`
	Type   string `json:"type"`
	Number string `json:"number"`
}

func NewContact(num, name string) Contact {
	return Contact{
		Type:   "phone",
		Number: num,
		Name:   name,
	}
}

type ContactsProvider interface {
	ReadBroadcastList(dest io.Writer) error
	ReadWhitelist(dest io.Writer) error
}

var ErrCorruptedContacts = errors.New("contacts file read contains corrupted data, thus the result could be partial")

func DecodeContacts(f func(io.Writer) error) ([]Contact, error) {
	var buf bytes.Buffer
	if err := f(&buf); err != nil {
		return []Contact{}, err
	}

	r := csv.NewReader(&buf)

	// lines starting with # are considered comments
	r.Comment = rune('#')

	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("decode contacts: %v", err)
	}

	acc := make([]Contact, 0, len(recs))
	for _, rec := range recs {
		if len(rec) < 2 {
			// discard record
			continue
		}
		acc = append(acc, NewContact(rec[0], rec[1]))
	}
	if len(acc) != len(recs) {
		return acc, ErrCorruptedContacts
	}
	return acc, nil
}

func (c *Client) Call(p ContactsProvider, recName string) {
	contacts, err := DecodeContacts(p.ReadBroadcastList)
	if err != nil {
		if err == ErrCorruptedContacts {
			log.Printf("call: %v", err)
		} else {
			log.Printf("call error: %v", err)
			return
		}
	}

	log.Printf("client: contacts decoded: %d", len(contacts))

	for _, v := range contacts {
		go func(contact Contact) {
			// We can make up to three req/sec. Give it twice as
			// that time as deadline.
			n := len(contacts) / 3
			if n < 1 {
				n = 1
			}

			d := n * 2
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(d))
			defer cancel()

			log.Printf("calling %v, message: %v", contact.Name, recName)
			if err := c.call(ctx, contact, recName); err != nil {
				log.Printf("call error: %v", err)
			}
		}(v)
	}
}

func (c *Client) call(ctx context.Context, to Contact, recName string) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(&struct {
		To     []Contact `json:"to"`
		From   Contact   `json:"from"`
		Answer []string  `json:"answer_url"`
		Event  []string  `json:"event_url"`
	}{
		To: []Contact{to},
		From: Contact{
			Type:   "phone",
			Number: c.Number,
		},
		Answer: []string{c.Origin + "/play/recording/" + recName},
		Event:  []string{c.Origin + "/play/recording/event"},
	}); err != nil {
		return fmt.Errorf("unable to encode ncco: %v", err)
	}

	_, err := c.Post(ctx, "https://api.nexmo.com/v1/calls", &buf)
	if err != nil {
		return fmt.Errorf("unable to make call: %v", err)
	}

	return nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		return nil
	}
	return fmt.Errorf("request failed: %s", resp.Status)
}
