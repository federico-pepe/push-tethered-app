# remap

Forwards Push's controls to MIDI, same as `thru`, but every control can be
individually overridden by a user-edited rule — a persisted table of
`MidiMapping` rules (source → output CC/note, with scaling and
relative-encoder accumulation), ported from `hacks/push-manager`. With no
rules defined its behaviour is identical to `thru`.

Rules can be created and edited entirely on-device, or by hand-editing the
module's config file (the host logs its path on activation) as a JSON
object under `"overrides"`, keyed by source. Example — pad note 40 out as
note 45, velocity rescaled into 20-100:

```json
{
  "overrides": {
    "note:40": {"out_type":"note","out_ch":1,"out_num":45,"out_min":20,"out_max":100}
  }
}
```

```bash
go run ./cmd/pushapp -module remap
```
