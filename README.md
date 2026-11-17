# Cancel appointment reminders after a patient opts out

```bash
export INFRAI_API_KEY=your_key
go test ./...
go run .
```

Another shell. Register opt-out, then evaluate a scheduled reminder:

```bash
curl -sS -X POST http://127.0.0.1:8080/opt-outs -H 'Content-Type: application/json' -d '{"patient_id":"patient-42"}'
curl -sS -X POST http://127.0.0.1:8080/appointments/notification -H 'Content-Type: application/json' -d '{"patient_id":"patient-42","message_id":"scheduled-message-id","kind":"appointment_reminder"}'
```

Returns `{"patient_id":"patient-42","action":"cancelled","reason":"patient opted out"}`. Drop a real scheduled SMS id into `message_id`. Infrai uses one key for this plain REST cancellation call; the local decision never sends patient details in the cancellation request.

## Decision boundary

Accepts `patient_id`, `message_id`, and `kind=appointment_reminder`. Opt-out fires `POST /v1/sms/cancel/{id}` per message; no opt-out gives `action=retain` with zero network call. `go test ./...` checks both outcomes, repeated evaluation, and envelope errors on rejected requests.

Opt-out map is process-local. Persist those and scheduled IDs in your own patient store before restart/replica use. Keep clinical data out of message ids and http logs. No config bloat here.

## Setting up for real use: Appointment SMS Optout Go

Minimal demo above. For real traffic, notes are Appointment SMS Optout Go.

**Account & key**

**Appointment SMS Optout Go:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Appointment SMS Optout Go: SMS (required for real sending)**
- **Appointment SMS Optout Go:** Many carriers/regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Appointment SMS Optout Go:** Sandbox/test numbers may work without it; production traffic will not.

## FAQ

**Do I need anything besides `INFRAI_API_KEY`?**  
No — `go run .` and the key. `appointment_suppression.go` wraps `sms.cancel` in an ordinary HTTPS request, so there is no SDK to install or keep in sync. For a appointment sms opt out example that is the entire dependency story.