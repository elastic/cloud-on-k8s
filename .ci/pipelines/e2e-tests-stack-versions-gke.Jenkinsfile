// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

def failedTests = []
def lib

pipeline {

    agent {
        label 'linux'
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
        stage('Run tests for different Elastic Stack versions in GKE') {
            parallel {
                stage("6.8.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-68-${BUILD_NUMBER}-e2e", "6.8.23")
                        }
                    }
                }
                stage("7.13.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-713-${BUILD_NUMBER}-e2e", "7.13.4")
                        }
                    }
                }
                stage("7.14.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-714-${BUILD_NUMBER}-e2e", "7.14.2")
                        }
                    }
                }
                stage("7.15.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-715-${BUILD_NUMBER}-e2e", "7.15.2")
                        }
                    }
                }
                stage("7.16.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-716-${BUILD_NUMBER}-e2e", "7.16.3")
                        }
                    }
                }
                stage("7.17.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-717-${BUILD_NUMBER}-e2e", "7.17.8")
                        }
                    }
                }
                stage("8.0.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-80-${BUILD_NUMBER}-e2e", "8.0.1")
                        }
                    }
                }
                stage("8.1.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-81-${BUILD_NUMBER}-e2e", "8.1.3")
                        }
                    }
                }
                stage("8.2.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-82-${BUILD_NUMBER}-e2e", "8.2.3")
                        }
                    }
                }
                stage("8.3.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-83-${BUILD_NUMBER}-e2e", "8.3.3")
                        }
                    }
                }
                stage("8.4.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-84-${BUILD_NUMBER}-e2e", "8.4.3")
                        }
                    }
                }
                stage("8.5.x") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        unstash "source"
                        script {
                            runWith(lib, failedTests, "eck-85-${BUILD_NUMBER}-e2e", "8.5.3")
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

                    slackSend(
                        channel: '#cloud-k8s',
                        color: 'danger',
                        message: lib.generateSlackMessage("E2E tests for different Elastic stack versions failed!", env.BUILD_URL, filter),
                        tokenCredentialId: 'cloud-ci-slack-integration-token',
                        failOnError: true
                    )
                }
                googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                    credentialsId: "devops-ci-gcs-plugin",
                    pattern: "*.zip",
                    sharedPublicly: true,
                    showInline: true
            }
        }
        cleanup {
            script {
                clusters = [
                    "eck-68-${BUILD_NUMBER}-e2e",
                    "eck-713-${BUILD_NUMBER}-e2e",
                    "eck-714-${BUILD_NUMBER}-e2e",
                    "eck-715-${BUILD_NUMBER}-e2e",
                    "eck-716-${BUILD_NUMBER}-e2e",
                    "eck-717-${BUILD_NUMBER}-e2e",
                    "eck-80-${BUILD_NUMBER}-e2e",
                    "eck-81-${BUILD_NUMBER}-e2e",
                    "eck-82-${BUILD_NUMBER}-e2e",
                    "eck-83-${BUILD_NUMBER}-e2e",
                    "eck-84-${BUILD_NUMBER}-e2e",
                    "eck-85-${BUILD_NUMBER}-e2e"
                ]
                for (int i = 0; i < clusters.size(); i++) {
                    build job: 'cloud-on-k8s-e2e-cleanup',
                        parameters: [string(name: 'JKS_PARAM_GKE_CLUSTER', value: clusters[i])],
                        wait: false
                }
            }
            cleanWs()
        }
    }

}

def runWith(lib, failedTests, clusterName, stackVersion) {
    sh ".ci/setenvconfig e2e/stack-versions $stackVersion $clusterName"
    script {
        env.SHELL_EXIT_CODE = sh(returnStatus: true, script: "make -C .ci get-test-artifacts TARGET=ci-e2e ci")

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
