# hello-js

The same module as `hello-py`, in Node — proving the process-loader
protocol isn't Python-specific. Press a pad, the screen shows which one
and a pad lights up to match. Nothing more.

```bash
go run ./cmd/pushapp -install examples/modules/hello-js
go run ./cmd/pushapp -module hello-js
```
