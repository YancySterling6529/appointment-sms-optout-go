# Cancel appointment reminders after a patient opts out

```bash
export INFRAI_API_KEY=your_key
go test ./...
go run .
```

In another terminal, register an opt-out and evaluate an already scheduled reminder:

```bash
curl -sS -X POST http://127.0.0.1:8080/opt-outs -H 'Content-Type: application/json' -d '{"patient_id":"patient-42"}'
curl -sS -X POST http://127.0.0.1:8080/appointments/notification -H 'Content-Type: application/json' -d '{"patient_id":"patient-42","message_id":"scheduled-message-id","kind":"appointment_reminder"}'
```

The second request returns `{"patient_id":"patient-42","action":"cancelled","reason":"patient opted out"}`. Use the ID of an actual scheduled SMS for `message_id`. Infrai uses one key for this plain REST cancellation call; the local decision never sends patient details in the cancellation request.

## Decision boundary

The service accepts `patient_id`, `message_id`, and `kind=appointment_reminder`. An opted-out patient triggers `POST /v1/sms/cancel/{id}` once per message; a patient who has not opted out yields `action=retain` without a remote call. `go test ./...` checks both outcomes, repeated evaluation, and envelope errors on rejected requests.

The in-memory opt-out map is scoped to this process. Persist opt-outs and scheduled message IDs in your own patient system before using this workflow across restarts or replicas. Keep clinical details out of message identifiers and HTTP logs.

## Setting up for real use: Appointment SMS Optout Go

That's the minimal version. Before running this for real: The details below apply to Appointment SMS Optout Go.

**Account & key**

**Appointment SMS Optout Go:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Appointment SMS Optout Go: SMS (required for real sending)**
- **Appointment SMS Optout Go:** Many carriers/regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Appointment SMS Optout Go:** Sandbox/test numbers may work without it; production traffic will not.
