# Elastic K8S Operators Contributing Guide

# Welcome !

Thank you for your interest in contributing to the Elastic K8S operator !
The goal of this document is to provide a high-level overview of how you can get involved.

-   [Bug reports](#bug-reports)
-   [Development environment](#development-environment)
-   [Contributing code](#contributing-code)
    -   [Format your code and import management](#format-your-code-and-import-management)
    -   [Test suites](#test-suites)
    -   [Logging](#logging)
    -   [License Headers](#license-headers)
-   [Submitting changes](#submitting-changes)
    -   [Sign the CLA](#sign-the-cla)
    -   [Prepare a Pull Request](#prepare-a-pull-request)
-   [Design documents](#design-documents)

## Bug reports
If you think you have found an issue please first check in our [issue list](https://github.com/elastic/k8s-operators/issues) that your problem has not already been reported.
If not please open a new one with a detailed report. It is always appreciated when a report includes some steps to reproduce the problem or contains any additional information that may help resolving the issue.

## Development environment
Check out this [README](https://github.com/elastic/k8s-operators/blob/master/operators/README.md) to learn about setting up your development environment.

## Contributing code

### Format your code and manage imports
Please run your code through [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports) and [go vet](https://golang.org/cmd/vet/) before opening a pull request. Also ensure that there are only two groups in your imports: one for packages from the standard library and an other one for third parties.

### Scripts
As most of the contributors are using macOS and Linux please ensure that scripts run on these two environments.

### Tests
We all know how important testing our code is, contributions should pass existing tests and new tests should be provided to demonstrate bugs and fixes.

#### Test suites
There are 3 test suites:
* **Unit tests** - use standard `go test` and [github.com/stretchr/testify/assert](https://github.com/stretchr/testify) assertions. Please keep them small, fast and reliable.
  
  Unit test must be [table-driven tests](https://github.com/golang/go/wiki/TableDrivenTests), you can use [gotests](https://github.com/cweill/gotests) to quickly generate them from your code.
  
* **Integration tests** - almost like unit tests but they rely on Kubebuilder to start a local control plane.
* **End-to-end tests** (e2e) allow us to test interactions between the operator and a real Kubernetes cluster. They use the standard `go test` tooling, please see the `test/e2e` directory. Keep in mind that while they simulate some real user scenarios end-to-end tests are slow and hard to debug, we should rely primarily on unit and integration tests.

### Logging
The operator relies on controller-runtime logging instead of golang built-in log library. It uses a kind of logging called structured logging, log messages must not contain variables but they can be associated with some key/value pairs.

For instance, do not write:
```
log.Printf("starting reconciliation for pod %s/%s", podNamespace, podName)
```

But instead write:
```
logger.Info("starting reconciliation", "pod", req.NamespacedNamed)
```

We only use two levels: `debug` and `info`. To produce a log at the `debug` level use `V(1)` before the `Info` call:
```
logger.V(1).Info("starting reconciliation", "pod", req.NamespacedNamed)
```

### License Headers
We require license headers on all files that are part of the source code.

### Submitting changes

#### Sign the CLA
Please make sure you have signed our [Contributor License Agreement](https://www.elastic.co/fr/contributor-agreement/). We are not asking you to assign copyright to us, but to give us the right to distribute your code without restriction. We ask this of all contributors in order to assure our users of the origin and continuing existence of the code. You only need to sign the CLA once.

#### Prepare a Pull Request
A good pull request will be reviewed quickly, here are some simples rules:
* Push your changes to a topic branch in your fork of the repository.
* If you think that your pull request is too large do not hesitate to break it down into smaller ones.
* Run and pass unit and integration tests with `make unit` and `make integration` 
* Title should be short and self-explanatory.
* The code reviewer should be able to understand what your pull request is doing by reading the description.

## Design documents
We keep track of architectural decisions through some [architectural decision records](https://adr.github.io/). All records must respect the [Markdown Architectural Decision Records](https://adr.github.io/madr/) format. They are available [here](https://github.com/elastic/k8s-operators/tree/master/docs/design) and we encourage you to read these documents to understand some of the technical choices that have been made. 

# Thank you !
Thank you for taking the time to contribute.