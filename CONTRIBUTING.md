# Contributing to Real-time Driver Location Tracker

First off, thank you for considering contributing to the Real-time Driver Location Tracker project! It's because of people like you that this tool continues to improve for everyone.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct (standard contributor covenant).

## How Can I Contribute?

### Reporting Bugs
If you find a bug, please open an issue and include:
*   A clear and descriptive title.
*   Steps to reproduce.
*   The expected vs. actual behavior.
*   Details about your environment (OS, Go version, Docker version).

### Suggesting Enhancements
We're always looking for new ideas! To suggest an enhancement:
*   Open an issue with the [Enhancement] tag.
*   Explain the intended use case and why this is a valuable addition.

### Pull Requests
Ready to contribute code? Follow these steps:
1.  Fork the repository.
2.  Create a new branch for your feature or fix.
3.  Write clean, idiomatic Go code.
4.  Ensure all tests are passing.
5.  Document your changes in the `README.md` if necessary.
6.  Submit a Pull Request with a clear description of the work.

## Development Setup

See the [SETUP.md](./SETUP.md) for detailed instructions on setting up your local environment.

## Architecture

The system uses a high-throughput pipeline (Kafka -> Go -> Cassandra/Redis). If you're adding new services, please ensure they follow the established pattern of decoupling via Kafka.
You can view and modify the system's architecture diagram using the **draw.io** extension:
*   Open [assets/architecture.drawio](./assets/architecture.drawio) in VS Code.

## License

By contributing, you agree that your contributions will be licensed under its **GNU GPL v3.0** License.
