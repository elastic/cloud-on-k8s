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
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
    }

    stages {
        stage('Checkout from GitHub') {
            steps {
                checkout scm
            }
        }
        stage('Load common scripts') {
            steps {
                script {
                    lib = load ".ci/common/tests.groovy"
                }
            }
        }
        stage('Run Checks') {
            when {
                expression {
                    notOnlyDocs()
                }
            }
            steps {
                sh 'make -C .ci TARGET=ci-check ci'
            }
        }
        stage("E2E tests") {
            when {
                expression {
                    notOnlyDocs()
                }
            }
            steps {
                sh '.ci/setenvconfig e2e/master'
                script {
                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-artifacts TARGET=ci-build-operator-e2e-run ci')

                    sh 'make -C .ci TARGET=e2e-generate-xml ci'
                    junit "e2e-tests.xml"

                    if (env.SHELL_EXIT_CODE != 0) {
                        failedTests = lib.getListOfFailedTests()

                        googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                            credentialsId: "devops-ci-gcs-plugin",
                            pattern: "*.tgz",
                            sharedPublicly: true,
                            showInline: true
                    }

                    sh 'exit $SHELL_EXIT_CODE'
                }
            }
        }
    }

    post {
        unsuccessful {
            script {
                def msg = lib.generateSlackMessage("E2E tests failed!", env.BUILD_URL, failedTests)

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
        cleanup {
            script {
                if (notOnlyDocs()) {
                    build job: 'cloud-on-k8s-e2e-cleanup',
                        parameters: [string(name: 'JKS_PARAM_GKE_CLUSTER', value: "eck-e2e-${BUILD_NUMBER}")],
                        wait: false
                }
            }

            cleanWs()
        }
    }
}

def notOnlyDocs() {
    // grep succeeds if there is at least one line without docs/
    return sh (
        script: "git diff --name-status HEAD~1 HEAD | grep -v docs/",
    	returnStatus: true
    ) == 0
}
