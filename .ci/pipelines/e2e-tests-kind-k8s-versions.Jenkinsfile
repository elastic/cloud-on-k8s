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
                            runTests(lib, failedTests, "kindest/node:v1.14.10@sha256:ce4355398a704fca68006f8a29f37aafb49f8fc2f64ede3ccd0d9198da910146", "0.9.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.15.12@sha256:d9b939055c1e852fe3d86955ee24976cab46cba518abcb8b13ba70917e6547a6", "0.9.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.16.15@sha256:a89c771f7de234e6547d43695c7ab047809ffc71a0c3b65aa54eda051c45ed20", "0.9.0", "ipv4")
                        }
                    }
                }
                stage("1.17.11") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.17.11@sha256:5240a7a2c34bf241afb54ac05669f8a46661912eab05705d660971eeb12f6555", "0.9.0", "ipv4")
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
                            runTests(lib, failedTests, "kindest/node:v1.18.8@sha256:f4bcc97a0ad6e7abaf3f643d890add7efe6ee4ab90baeb374b4f41a4c95567eb", "0.9.0", "ipv4")
                        }
                    }
                }
                stage("1.19.1") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.19.1@sha256:98cf5288864662e37115e362b23e4369c8c4a408f99cbc06e58ac30ddc721600", "0.9.0", "ipv4")
                        }
                    }
                }
                stage("1.20.0 IPv4") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.20.0@sha256:b40ecf8bcb188f6a0d0f5d406089c48588b75edc112c6f635d26be5de1c89040", "0.9.0", "ipv4")
                        }
                    }
                }
                stage("1.20.0 IPv6") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        unstash "source"
                        script {
                            runTests(lib, failedTests, "kindest/node:v1.20.0@sha256:b40ecf8bcb188f6a0d0f5d406089c48588b75edc112c6f635d26be5de1c89040", "0.9.0", "ipv6")
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

def runTests(lib, failedTests, kindNodeImage, kindVersion, ipFamily) {
    sh ".ci/setenvconfig e2e/kind-k8s-versions $kindNodeImage $kindVersion $ipFamily"
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
