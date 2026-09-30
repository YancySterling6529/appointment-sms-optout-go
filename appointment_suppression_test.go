package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeSMS struct{ calls int }

func (f *fakeSMS) Cancel(_ context.Context, _ string) error { f.calls++; return nil }

func TestAppointmentDecision(t *testing.T) {
	for _, tc := range []struct {
		name   string
		optOut bool
		want   string
		calls  int
	}{
		{"allowed", false, "retain", 0},
		{"opted out", true, "cancelled", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeSMS{}
			s := &service{optedOut: map[string]bool{"p1": tc.optOut}, cancelled: map[string]bool{}, sms: fake}
			item := appointment{PatientID: "p1", MessageID: "m1", Kind: "appointment_reminder"}
			got, err := s.process(context.Background(), item)
			if err != nil || got.Action != tc.want || fake.calls != tc.calls {
				t.Fatalf("decision=%+v calls=%d err=%v", got, fake.calls, err)
			}
			if tc.optOut {
				_, err = s.process(context.Background(), item)
				if err != nil || fake.calls != 1 {
					t.Fatalf("repeated cancellation: calls=%d err=%v", fake.calls, err)
				}
			}
		})
	}
}

func TestCancelEnvelopeBeforeStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sms/cancel/m1" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" || body["message_id"] != "m1" || body["idempotency_key"] == "" || len(body) != 2 {
			t.Errorf("unexpected request: %+v", r)
		}
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false,"error":{"code":"BAD_REQUEST","hint":"invalid id"}}`))
	}))
	defer server.Close()
	err := (smsClient{key: "test-key", http: server.Client(), base: server.URL}).Cancel(context.Background(), "m1")
	if err == nil || !strings.Contains(err.Error(), "invalid id") {
		t.Fatalf("want envelope error, got %v", err)
	}
}
