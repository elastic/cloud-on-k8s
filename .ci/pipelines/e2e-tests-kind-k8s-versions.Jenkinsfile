// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

def failedTests = []
def lib

pipeline {

    agent {
        label 'eck'
    }

    options {
        timeout(time: 600, unit: 'MINUTES')
        skipDefaultCheckout(true)
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
    }

    stages {
        stage('Checkout, stash source code and load common scripts') {
            steps {
                checkout scm
                stash allowEmpty: true, name: 'source', useDefaultExcludes: false
                script {
                    lib = load ".ci/common/tests.groovy"
                }
            }
        }
        stage('Run tests on different versions of vanilla K8s') {
            // Do not forget to keep in sync the kind node image versions in `.ci/packer_cache.sh`.
            parallel {
                stage("1.19.11") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.19.11@sha256:07db187ae84b4b7de440a73886f008cf903fcf5764ba8106a9fd5243d6f32729", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.20.7") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.21.1") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.21.1@sha256:69860bda5563ac81e3c0057d654b5253219618a22ec3a346306239bba8cfa1a6", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.22.0 IPv4") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.22.0@sha256:b8bda84bb3a190e6e028b1760d277454a72267a5454b57db34437c34a588d047", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.22.0 IPv6") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.22.0@sha256:b8bda84bb3a190e6e028b1760d277454a72267a5454b57db34437c34a588d047", "0.11.1", "ipv6")
                        }
                    }
                }
                stage("1.23.3 IPv4") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.23.3@sha256:0df8215895129c0d3221cda19847d1296c4f29ec93487339149333bd9d899e5a", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.23.3 IPv6") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.23.3@sha256:0df8215895129c0d3221cda19847d1296c4f29ec93487339149333bd9d899e5a", "0.11.1", "ipv6")
                        }
                    }
                }
            }
        }
    }

    post {
        unsuccessful {
            script {
                if (params.SEND_NOTIFICATIONS) {
                    Set<String> filter = new HashSet<>()
                    filter.addAll(failedTests)
                    def msg = lib.generateSlackMessage("E2E tests for different versions of vanilla K8s failed!", env.BUILD_URL, filter)

                    slackSend(
                        channel: '#cloud-k8s',
                        color: 'danger',
                        message: msg,
                        tokenCredentialId: 'cloud-ci-slack-integration-token',
                        failOnError: true
                    )
                }
            }
        }
        cleanup {
            cleanWs()
        }
    }

}

def runTests(lib, failedTests, kindNodeImage, kindVersion, ipFamily) {
    sh ".ci/setenvconfig e2e/kind-k8s-versions $kindNodeImage $kindVersion $ipFamily"
    script {
        env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-artifacts TARGET=ci-e2e ci')

        sh 'make -C .ci TARGET=e2e-generate-xml ci'
        junit "e2e-tests.xml"

        if (env.SHELL_EXIT_CODE != 0) {
            failedTests.addAll(lib.getListOfFailedTests())
            googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                credentialsId: "devops-ci-gcs-plugin",
                pattern: "*.zip",
                sharedPublicly: true,
                showInline: true
        }

        sh 'exit $SHELL_EXIT_CODE'
    }
}
