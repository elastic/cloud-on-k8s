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
                stage("1.12.10") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.12.10", "1.12", "ipv4")
                        }
                    }
                }
                stage("1.13.12") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.13.12", "1.13", "ipv4")
                        }
                    }
                }
                stage("1.14.10") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.14.10", "1.14", "ipv4")
                        }
                    }
                }
                stage("1.15.11") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.15.11", "1.15", "ipv4")
                        }
                    }
                }
                stage("1.16.9") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.16.9", "1.16", "ipv4")
                        }
                    }
                }
                stage("1.17.5") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.17.5", "1.17", "ipv4")
                        }
                    }
                }
                stage("1.18.2 IPv4") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.18.2", "1.18", "ipv4")
                        }
                    }
                }
                stage("1.18.2 IPv6") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.18.2", "1.18", "ipv6")
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
                        botUser: true,
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

def runTests(lib, failedTests, kindNodeImage, clusterVersion, ipFamily) {
    sh ".ci/setenvconfig e2e/kind-k8s-versions $kindNodeImage $clusterVersion $ipFamily"
    script {
        env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-artifacts TARGET=kind-e2e ci')

        sh 'make -C .ci TARGET=e2e-generate-xml ci'
        junit "e2e-tests.xml"

        if (env.SHELL_EXIT_CODE != 0) {
            failedTests.addAll(lib.getListOfFailedTests())
            googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                credentialsId: "devops-ci-gcs-plugin",
                pattern: "*.tgz",
                sharedPublicly: true,
                showInline: true
        }

        sh 'exit $SHELL_EXIT_CODE'
    }
}
