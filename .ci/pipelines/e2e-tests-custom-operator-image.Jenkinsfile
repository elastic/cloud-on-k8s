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
        stage("Run E2E tests") {
            steps {
                script {
                    sh '.ci/setenvconfig e2e/custom-operator-image'
                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-license get-elastic-public-key TARGET=ci-e2e ci')

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
                if (params.SEND_NOTIFICATIONS) {
                    def msg = testScript.generateSlackMessage("E2E tests failed!", env.BUILD_URL, failedTests)

                    slackSend botUser: true,
                        channel: '#cloud-k8s',
                        color: 'danger',
                        message: msg,
                        tokenCredentialId: 'cloud-ci-slack-integration-token'
                }
            }
        }
        cleanup {
            build job: 'cloud-on-k8s-e2e-cleanup',
                parameters: [string(name: 'JKS_PARAM_GKE_CLUSTER', value: "eck-e2e-${BUILD_NUMBER}")],
                wait: false
            cleanWs()
        }
    }

}
