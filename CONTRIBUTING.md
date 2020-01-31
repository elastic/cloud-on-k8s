# How to Contribute to Elastic Cloud on Kubernetes

Thank you for your interest in contributing to Elastic Cloud on Kubernetes!
The goal of this document is to provide a high-level overview on how you can get involved.

- [How to Contribute to Elastic Cloud on Kubernetes](#how-to-contribute-to-elastic-cloud-on-kubernetes)
  - [Report your bugs](#report-your-bugs)
  - [Set up your development environment](#set-up-your-development-environment)
  - [Contribute with your code](#contribute-with-your-code)
    - [Format your code and manage imports](#format-your-code-and-manage-imports)
    - [Scripts](#scripts)
    - [Tests](#tests)
      - [Test suites](#test-suites)
    - [Logging](#logging)
    - [License headers](#license-headers)
    - [Submit your changes](#submit-your-changes)
      - [Sign the CLA](#sign-the-cla)
      - [Prepare a Pull Request](#prepare-a-pull-request)
  - [Contribute to ECK documentation](#contribute-to-eck-documentation)
  - [Design documents](#design-documents)

## Report your bugs

If you find an issue, check first our [list of issues](https://github.com/elastic/cloud-on-k8s/issues). If your problem has not been reported yet, open a new issue, add a detailed description on how to reproduce the problem and complete it with any additional information that might help solving the issue.

## Set up your development environment

Check requirements and steps in this [guide](dev-setup.md).

## Contribute with your code

### Format your code and manage imports

1. Run `make lint` to make sure there are no lint warnings.
2. Make sure you only have two groups in your imports:
    - a group for packages from the standard library
    - a group for third parties

### Scripts

As most of the contributors are using macOS and Linux, make sure that scripts run on these two environments.

### Tests

Your contributions should pass the existing tests. You must provide new tests to demonstrate bugs and fixes.

#### Test suites

There are 3 test suites:

- **Unit tests** - use standard `go test` and [github.com/stretchr/testify/assert](https://github.com/stretchr/testify) assertions. Keep them small, fast and reliable.
  
  A good practice is to have some [table-driven tests](https://github.com/golang/go/wiki/TableDrivenTests), you can use [gotests](https://github.com/cweill/gotests) to quickly generate them from your code.
  
- **Integration tests** - some tests are flagged as integration as they can take more than a few milliseconds to complete. It's usually recommended to separate them from the rest of the unit tests that run fast. Usually they include disk I/O operations, network I/O operations on a test port, or encryption computations. We also rely on the kubebuilder testing framework, that spins up etcd and the apiserver locally, and enqueues requests to a reconciliation function.

- **End-to-end tests** - (e2e) allow us to test interactions between the operator and a real Kubernetes cluster.
      They use the standard `go test` tooling. See the `test/e2e` directory. We recommend to rely primarily on unit and integration tests, as e2e tests are slow and hard to debug because they simulate real user scenarios.

### Logging

The operator relies on controller-runtime logging instead of golang built-in log library. It uses a type of logging called _structured logging_, log messages must not contain variables, but they can be associated with some key/value pairs.

For example, do not write:

```golang
log.Printf("starting reconciliation for pod %s/%s", podNamespace, podName)
```

But instead write:

```golang
logger.Info("starting reconciliation", "pod", req.NamespacedNamed)
```

We only use two levels: `debug` and `info`. To produce a log at the `debug` level use `V(1)` before the `Info` call:

```golang
logger.V(1).Info("starting reconciliation", "pod", req.NamespacedNamed)
```

### License headers

We require license headers on all files that are part of the source code.

### Submit your changes

#### Sign the CLA

Make sure you signed the [Contributor License Agreement](https://www.elastic.co/fr/contributor-agreement/). You only need to sign the CLA once. By signing this agreement, you give us the right to distribute your code without restriction.

#### Prepare a Pull Request

Here are some good practices for a good pull request:

- Push your changes to a topic branch in your fork of the repository.
- Break your pull request into smaller PRs if it's too large.
- Run and pass unit and integration tests with `make unit` and `make integration`.
- Write a short and self-explanatory title.
- Write a clear description to make the code reviewer understand what the PR is about.

## Contribute to ECK documentation

Whether it’s a new or an existing feature, make sure you document it.

Before you start, pull the latest files from these repos:
[elastic/cloud-on-k8s](https://github.com/elastic/cloud-on-k8s): Contains the docs source files for Elastic Cloud on Kubernetes.
[elastic/docs](https://github.com/elastic/docs): Has the tools to publish locally your changes before committing them.

**To make a doc change**
1. Find the right file (or create a new one).
1. Add your content.
1. Test the doc build locally:
`$GIT_HOME/docs/build_docs --asciidoctor --doc $GIT_HOME/cloud-on-k8s/docs/index.asciidoc --chunk 1 —open`
1. Push a PR for review and add the label `>docs`.

**When a release goes out**
1. Track your doc issues in the Cloud Working Board with `>docs` and the release label.
1. Sync with the ECK release manager on which branch you push the doc PRs.
1. Update the [conf.yaml](https://github.com/elastic/docs/blob/master/conf.yaml) file:
   * Add the release branch to the list of `branches`. 
   * Set `current` to the release branch.

NOTE: Cloud documentation builds [roughly every 30 minutes](https://elasticsearch-ci.elastic.co/job/elastic+docs+master+build/). 

**To get in touch with a writer**
* Slack: ping **@cloud-writers** in [#cloud-pubs](https://elastic.slack.com/messages/C9HNMNZU1)
* GitHub: **@elastic/cloud-writers** and `area:doc` label 
* Email: cloud-pubs@elastic.co

**For more doc guidance**
* [How do I make a simple docs change?](https://github.com/elastic/cloud/blob/master/docs/README.md#how-do-i-make-a-simple-docs-change)
* [Doc build process](https://github.com/elastic/docs/blob/master/README.asciidoc)


## Design documents

We keep track of architectural decisions through the [architectural decision records](https://adr.github.io/). All records must apply the [Markdown Architectural Decision Records](https://adr.github.io/madr/) format. We recommend to read [these documents](https://github.com/elastic/cloud-on-k8s/tree/master/docs/design) to understand the technical choices that we make.

Thank you for taking the time to contribute.
