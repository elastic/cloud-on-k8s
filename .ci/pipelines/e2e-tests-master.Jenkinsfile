def failedTests = []
def testScript

pipeline {

    agent {
        label 'linux'
    }

    options {
        timeout(time: 300, unit: 'MINUTES')
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
                    testScript = load ".ci/common/tests.groovy"
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
                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-monitoring-secrets get-test-license get-elastic-public-key TARGET="build-operator-image ci-e2e" ci')

                    sh 'make -C .ci TARGET=e2e-generate-xml ci'
                    junit "e2e-tests.xml"

                    if (env.SHELL_EXIT_CODE != 0) {
                        failedTests = testScript.getListOfFailedTests()
                    }

                    sh 'exit $SHELL_EXIT_CODE'
                }
            }
        }
    }

    post {
        unsuccessful {
            script {
                def msg = testScript.generateSlackMessage("E2E tests failed!", env.BUILD_URL, failedTests)

                slackSend(
                      channel: '#cloud-k8s',
                      color: 'danger',
                      message: msg,
                    tokenCredentialId: 'cloud-ci-slack-integration-token',
                    botUser: true,
                    failOnError: false
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
