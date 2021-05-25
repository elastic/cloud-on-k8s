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
                            runTests(lib, failedTests, "kindest/node:v1.14.10@sha256:6033e04bcfca7c5f2a9c4ce77551e1abf385bcd2709932ec2f6a9c8c0aff6d4f", "0.11.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.15.12@sha256:8d575f056493c7778935dd855ded0e95c48cb2fab90825792e8fc9af61536bf9", "0.11.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.16.15@sha256:430c03034cd856c1f1415d3e37faf35a3ea9c5aaa2812117b79e6903d1fc9651", "0.11.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.17.17@sha256:c581fbf67f720f70aaabc74b44c2332cc753df262b6c0bca5d26338492470c17", "0.11.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.18.19@sha256:530378628c7c518503ade70b1df698b5de5585dcdba4f349328d986b8849b1ee", "0.11.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.19.11@sha256:7664f21f9cb6ba2264437de0eb3fe99f201db7a3ac72329547ec4373ba5f5911", "0.11.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.20.7@sha256:e645428988191fc824529fd0bb5c94244c12401cf5f5ea3bd875eb0a787f0fe9", "0.11.0", "ipv4")
                        }
                    }
                }
                stage("1.21.1 IPv4") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.21.1@sha256:fae9a58f17f18f06aeac9772ca8b5ac680ebbed985e266f711d936e91d113bad", "0.11.0", "ipv4")
                        }
                    }
                }
                stage("1.21.0 IPv6") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.21.1@sha256:fae9a58f17f18f06aeac9772ca8b5ac680ebbed985e266f711d936e91d113bad", "0.11.0", "ipv6")
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
