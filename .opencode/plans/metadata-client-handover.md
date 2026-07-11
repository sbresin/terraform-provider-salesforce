# Metadata Client Implementation - Handover

## Quick Start

Execute the implementation plan using subagent-driven development:

```
Execute docs/superpowers/plans/2026-07-11-metadata-client.md using subagent-driven development
```

## Context

We're building a Terraform provider for Salesforce that can upload JKS keystores to Certificate and Key Management. This plan implements the backend infrastructure:

1. **Session Auth Bridge** - Exchange OAuth tokens for UI session cookies
2. **Metadata Client** - SOAP API for certificate read/list/delete  
3. **Uploader** - UI form POST for certificate creation
4. **Provider Integration** - Wire everything together

## Plan Location

`docs/superpowers/plans/2026-07-11-metadata-client.md` (2694 lines, 10 tasks)

## Task Overview

| # | Task | Est. Time |
|---|------|-----------|
| 1 | Session Auth Bridge (`internal/client/session.go`) | 5 min |
| 2 | Metadata Types (`internal/metadata/types.go`) | 3 min |
| 3 | Metadata Errors (`internal/metadata/errors.go`) | 2 min |
| 4 | Metadata Client Core (`internal/metadata/client.go`) | 5 min |
| 5 | Certificate Read/List (`internal/metadata/certificate.go`) | 5 min |
| 6 | Deploy Operations (`internal/metadata/deploy.go`) | 5 min |
| 7 | Certificate Delete (add to `certificate.go`) | 3 min |
| 8 | Certificate Uploader (`internal/uploader/uploader.go`) | 5 min |
| 9 | Provider Integration (update `provider.go`) | 5 min |
| 10 | Data Source Update (update `certificate_data_source.go`) | 5 min |

## Key Files After Implementation

```
internal/
├── client/
│   ├── session.go         # NEW: SessionSource interface
│   └── session_test.go    # NEW
├── metadata/              # NEW PACKAGE
│   ├── types.go           # SOAP structs
│   ├── errors.go          # ErrNotFound, SOAPFaultError
│   ├── client.go          # NewClient(), doRequest()
│   ├── certificate.go     # ReadCertificate(), ListCertificates(), DeleteCertificate()
│   ├── deploy.go          # Deploy(), CheckDeployStatus()
│   └── *_test.go          # Unit tests
├── uploader/              # NEW PACKAGE
│   ├── uploader.go        # NewUploader(), Upload()
│   └── uploader_test.go
└── provider/
    ├── provider.go        # MODIFIED: ProviderClients struct
    └── certificate_data_source.go  # MODIFIED: uses metadata client
```

## Test Commands

```bash
# Run all tests
go test -v ./internal/...

# Run specific package tests
go test -v ./internal/client/ -run TestSessionSource
go test -v ./internal/metadata/
go test -v ./internal/uploader/
go test -v ./internal/provider/

# Lint
make lint
```

## Reference Documents

- `docs/certificate-upload-spec.md` - Full specification (auth flow, upload flow, API details)
- `poc/upload_certificate.py` - Working Python POC for reference

## Notes

- API version: Metadata API uses `67.0` (no `v` prefix)
- Logging: Uses `context.Context` with tflog (no custom logger interface needed)
- Session: Fresh session per call, no caching
- All code is TDD - tests first, then implementation
