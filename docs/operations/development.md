# Development

## Requirements

Requires Go 1.22+.

## Testing

Run the unit tests with:

```bash
go test ./...
```

## Pre-commit Hooks

Install the [pre-commit](https://pre-commit.com) tool and run:

```bash
pre-commit install
```

This installs Git hooks that automatically run formatting, linting, unit tests and a Docker build before each commit.

## Customization: Tailor the Ad Server to Your Needs

This ad server is built for deep customization, empowering publishers to control their ad serving logic precisely.

Ad selection is defined by the `selectors.Selector` interface. This pluggable architecture means you're not locked into a single way of choosing ads. The built-in `RuleBasedSelector` enforces priority, targeting, and pacing rules, serving as a robust default. For demonstration, a simple `RandomSelector` is included in `internal/logic/selectors/random.go`, showcasing how easily alternative strategies can be implemented and integrated.

To further support custom selector development, common helper functions are available in `internal/logic/filters`. These reusable filters handle essential tasks like targeting validation, ad size checks, frequency capping, and pacing enforcement. This modular approach allows new selector implementations to leverage existing, tested logic, significantly simplifying the development of bespoke ad selection strategies. This focus on extensibility ensures the ad server can adapt to unique publisher requirements and evolving market needs.
