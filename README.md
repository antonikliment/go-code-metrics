# goclocbudget

`goclocbudget` is a `golangci-lint` module plugin that enforces a repository-wide Go implementation line budget using `gocloc`.

## Usage

Add the plugin to `.custom-gcl.yml`:

```yaml
version: v2.11.4
name: custom-golangci-lint
destination: .

plugins:
  - module: github.com/antonikliment/goclocbudget
    version: v0.1.0
```

Enable it in `.golangci.yml`:

```yaml
version: "2"

linters:
  enable:
    - goclocbudget

  settings:
    custom:
      goclocbudget:
        type: "module"
        description: "Enforces the implementation Go LOC budget using gocloc."
        settings:
          max-go-code-lines: 10000
          include-tests: false
          exclude-generated: true
          exclude-dirs:
            - vendor
            - .git
            - node_modules
            - app/dist
```

Build and run:

```bash
golangci-lint custom
./custom-golangci-lint run
```
