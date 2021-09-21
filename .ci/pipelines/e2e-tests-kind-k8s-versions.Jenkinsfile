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
                            runTests(lib, failedTests, "kindest/node:v1.12.10@sha256:faeb82453af2f9373447bb63f50bae02b8020968e0889c7fa308e19b348916cb", "0.8.1", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.13.12@sha256:214476f1514e47fe3f6f54d0f9e24cfb1e4cda449529791286c7161b7f9c08e7", "0.8.1", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.14.10@sha256:f8a66ef82822ab4f7569e91a5bccaf27bceee135c1457c512e54de8c6f7219f8", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.15.12") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.15.12@sha256:b920920e1eda689d9936dfcf7332701e80be12566999152626b2c9d730397a95", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.16.15") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.16.15@sha256:83067ed51bf2a3395b24687094e283a7c7c865ccc12a8b1d7aa673ba0c5e8861", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.17.17") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.17.17@sha256:66f1d0d91a88b8a001811e2f1054af60eef3b669a9a74f9b6db871f2f1eeed00", "0.11.1", "ipv4")
                        }
                    }
                }
                stage("1.18.19") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.18.19@sha256:7af1492e19b3192a79f606e43c35fb741e520d195f96399284515f077b3b622c", "0.11.1", "ipv4")
                        }
                    }
                }
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
                pattern: "*.tgz",
                sharedPublicly: true,
                showInline: true
        }

        sh 'exit $SHELL_EXIT_CODE'
    }
}
