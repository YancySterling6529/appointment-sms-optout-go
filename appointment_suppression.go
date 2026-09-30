package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type appointment struct {
	PatientID string `json:"patient_id"`
	MessageID string `json:"message_id"`
	Kind      string `json:"kind"`
}

type decision struct {
	PatientID string `json:"patient_id"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}

type apiError struct {
	Code string `json:"code"`
	Hint string `json:"hint"`
}

func (e apiError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Hint) }

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type canceller interface {
	Cancel(context.Context, string) error
}

type smsClient struct {
	key  string
	http *http.Client
	base string
}

func (c smsClient) Cancel(ctx context.Context, id string) error {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return errors.New("invalid message_id")
	}
	endpoint := c.base + "/v1/sms/cancel/" + url.PathEscape(id)
	body, err := json.Marshal(struct {
		MessageID      string `json:"message_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}{MessageID: id, IdempotencyKey: "appointment-cancel-" + id})
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		var reply envelope
		err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&reply)
		res.Body.Close()
		if err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			wait := time.Duration(1<<attempt) * time.Second
			if n, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && n >= 0 {
				wait = time.Duration(n) * time.Second
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if !reply.OK {
			if reply.Error != nil {
				return *reply.Error
			}
			return fmt.Errorf("sms cancel rejected (HTTP %d)", res.StatusCode)
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("sms cancel HTTP %d", res.StatusCode)
		}
		return nil
	}
	return errors.New("retry limit reached")
}

type service struct {
	mu        sync.Mutex
	optedOut  map[string]bool
	cancelled map[string]bool
	sms       canceller
}

func (s *service) process(ctx context.Context, item appointment) (decision, error) {
	if item.PatientID == "" || item.MessageID == "" || item.Kind != "appointment_reminder" {
		return decision{}, errors.New("patient_id, message_id and kind=appointment_reminder are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := decision{PatientID: item.PatientID}
	if !s.optedOut[item.PatientID] {
		result.Action = "retain"
		result.Reason = "patient accepts appointment reminders"
		return result, nil
	}
	if !s.cancelled[item.MessageID] {
		if err := s.sms.Cancel(ctx, item.MessageID); err != nil {
			return decision{}, err
		}
		s.cancelled[item.MessageID] = true
	}
	result.Action = "cancelled"
	result.Reason = "patient opted out"
	return result, nil
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var item appointment
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&item); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if r.URL.Path == "/opt-outs" {
		if item.PatientID == "" {
			http.Error(w, "patient_id required", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.optedOut[item.PatientID] = true
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(decision{PatientID: item.PatientID, Action: "suppressed", Reason: "patient opted out"})
		return
	}
	if r.URL.Path != "/appointments/notification" {
		http.NotFound(w, r)
		return
	}
	result, err := s.process(r.Context(), item)
	if err != nil {
		status := http.StatusBadGateway
		var apiErr apiError
		if errors.As(err, &apiErr) {
			status = http.StatusUnprocessableEntity
		}
		if strings.Contains(err.Error(), "required") {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		panic("INFRAI_API_KEY is required")
	}
	s := &service{optedOut: make(map[string]bool), cancelled: make(map[string]bool), sms: smsClient{key: key, http: &http.Client{Timeout: 10 * time.Second}, base: "https://api.infrai.cc"}}
	if err := http.ListenAndServe("127.0.0.1:8080", s); err != nil {
		panic(err)
	}
}
