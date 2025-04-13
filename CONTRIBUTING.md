# Contributing to Go on Airplanes

Thank you for your interest in contributing to Go on Airplanes! This document provides guidelines and instructions for contributing to this project.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md) to foster an inclusive and respectful community.

## Getting Started

### Prerequisites

- Go 1.16 or higher
- Basic knowledge of web development
- Familiarity with Git and GitHub

### Setting Up Your Development Environment

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```
   git clone https://github.com/YOUR-USERNAME/goonairplanes.git
   ```
3. Set up the original repository as an upstream remote:
   ```
   git remote add upstream https://github.com/kleeedolinux/goonairplanes.git
   ```
4. Install dependencies:
   ```
   go mod tidy
   ```

## Development Workflow

1. Create a new branch for your feature or bugfix:
   ```
   git checkout -b feature/your-feature-name
   ```
   or
   ```
   git checkout -b fix/your-bugfix-name
   ```

2. Make your changes, following our coding standards and guidelines

3. Run tests locally:
   ```
   go test ./...
   ```

4. Commit your changes with a descriptive commit message:
   ```
   git commit -m "Add feature: your feature description"
   ```

5. Push your branch to your fork:
   ```
   git push origin feature/your-feature-name
   ```

6. Open a Pull Request against the main repository

## Security Vulnerabilities

If you discover a security vulnerability within Go on Airplanes, please send an email to me@juliaklee.wtf. All security vulnerabilities will be promptly addressed.

Please do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.

## Adding API Routes

### File Structure

API routes should be organized in the following structure:
```
app/
  api/
    module-name/
      route.go
```

### Implementation Guidelines

1. Create a new directory under `app/api/` for your module
2. Create a `route.go` file in your module directory
3. Implement the API handler function

Example:
```go
package api

import (
	"goonairplanes/core"
	"net/http"
)

// Handler is the main entry point for this API route
func Handler(ctx *core.APIContext) {
	// Handle different HTTP methods
	switch ctx.Request.Method {
	case http.MethodGet:
		// Handle GET request
		handleGet(ctx)
	default:
		ctx.Error("Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGet(ctx *core.APIContext) {
	// Your implementation here
	ctx.Success(map[string]string{"message": "Success"}, http.StatusOK)
}
```

## Pull Request Process

1. Update the README.md and documentation with details of any changes to the interface
2. Ensure all tests pass and your code follows our coding standards
3. Request a review from one of the core team members
4. Once approved, your PR will be merged by a maintainer

## Coding Standards

- Follow Go's standard formatting and coding guidelines
- Use meaningful variable and function names
- Write comprehensive comments for complex logic
- Keep functions focused and small
- Add tests for new features

## Testing

- All new features should come with appropriate tests
- Ensure all tests pass before submitting a PR
- Add both unit tests and integration tests when applicable

## Documentation

- Update documentation for any changed functionality
- Use clear and concise language
- Provide examples when helpful
- Keep API documentation up-to-date

## Community

- Join our community discussions on [GitHub Discussions](https://github.com/kleeedolinux/goonairplanes/discussions)
- Ask questions and share knowledge
- Help others solve their issues
- Suggest improvements

We appreciate your contributions and look forward to your involvement in making Go on Airplanes better for everyone! 