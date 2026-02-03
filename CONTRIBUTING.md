# Contributing to MDEMG

Thank you for your interest in contributing to MDEMG (Multi-Dimensional Emergent Memory Graph). This document provides guidelines for contributing to the project.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Neo4j 5.x with vector index support
- An embedding provider (OpenAI API or Ollama)

### Development Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/reh3376/mdemg.git
   cd mdemg
   ```

2. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

3. Configure your `.env` file with:
   - Neo4j connection details
   - Embedding provider credentials

4. Start Neo4j (if using Docker):
   ```bash
   docker compose up -d
   ```

5. Build the server:
   ```bash
   go build -o bin/mdemg ./cmd/server
   ```

6. Run the server:
   ```bash
   ./bin/mdemg
   ```

## Code Style

- Follow standard Go conventions and idioms
- Run `go fmt` before committing
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Keep functions focused and reasonably sized

## Testing

- Write tests for new functionality
- Run existing tests before submitting:
  ```bash
  go test ./...
  ```
- Include both unit tests and integration tests where appropriate

## Submitting Changes

### Pull Request Process

1. Fork the repository and create a feature branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes following the code style guidelines

3. Write or update tests as needed

4. Commit your changes with clear, descriptive messages:
   ```bash
   git commit -m "feat: add new retrieval optimization"
   ```

   Use conventional commit prefixes:
   - `feat:` - New features
   - `fix:` - Bug fixes
   - `docs:` - Documentation changes
   - `test:` - Test additions or changes
   - `refactor:` - Code refactoring
   - `perf:` - Performance improvements

5. Push to your fork and open a Pull Request

6. Ensure CI checks pass and address any review feedback

### What to Include in a PR

- Clear description of the changes
- Motivation/context for the changes
- Any breaking changes noted
- Test plan or evidence of testing

## Project Structure

```
mdemg/
├── cmd/           # CLI entry points (server, ingest-codebase, etc.)
├── internal/      # Internal packages
│   ├── api/       # HTTP API handlers
│   ├── retrieval/ # Core retrieval algorithms
│   ├── hidden/    # Hidden layer and concept abstraction
│   ├── learning/  # Hebbian learning edges
│   ├── embeddings/# Embedding providers
│   └── ...
├── docs/          # Documentation
└── plugins/       # Plugin modules
```

## Reporting Issues

When reporting bugs, please include:

- Go version (`go version`)
- Neo4j version
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or error messages

## Code of Conduct

This project follows a Code of Conduct to ensure a welcoming and respectful environment for everyone. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before participating.

## License

By contributing to MDEMG, you agree that your contributions will be licensed under the MIT License.
