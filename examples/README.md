# Example Traces

Drop EEBus trace and log files into this directory for analysis with
EEBusTracer.

## Supported file formats

| Format | Extension | Description |
|--------|-----------|-------------|
| EEBusTracer trace | `.eet` | Native export format (JSON-based) |
| eebus-go log | `.log` | Log output from eebus-go / spine-go stacks |
| eebustester log | `.log` | Log output from eebustester (EEBus Living Lab); auto-detected |

## Usage

**Web UI** — Start the server and import files via drag-and-drop on the welcome
page or the "Load from file" button:

```bash
go run ./cmd/eebustracer serve
# open http://localhost:8080
```

**CLI import** — Import a file directly into the database:

```bash
go run ./cmd/eebustracer import examples/my-trace.eet
go run ./cmd/eebustracer import examples/eebustester.log
```

**CLI analyze** — Run protocol analysis without importing:

```bash
go run ./cmd/eebustracer analyze examples/my-trace.eet --check all
```

## Notes

- Files in this directory are git-ignored (except this README)
- You can organize files in subdirectories if needed
