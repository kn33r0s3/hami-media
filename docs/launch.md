# Hami Media

Hami Media is a self-hosted Go live-streaming media server.

It explores WebRTC, browser viewers, connection recovery,
authentication, and TURN in a single project.

## Build

    go build -o hami-media .

## Test

    go test ./...

## Documentation

- README.md
- docs/setup.md
- docs/configuration.md
- docs/integration.md
- docs/capacity.md
- REPORT.md

## Engineering transparency

Performance depends on hardware, network conditions, configuration,
and workload. Measurements apply to their documented test conditions,
not universal capacity guarantees.

Reproduce the tests and report differences.

## Contributions

Bug reports, reproducible benchmarks, code review, and documentation
improvements are welcome. See CONTRIBUTING.md.
