def failedTests = []
def testScript

pipeline {

    agent {
        label 'eck'
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
        stage('Load common scripts') {
            steps {
                script {
                    testScript = load ".ci/common/tests.groovy"
                }
            }
        }
        stage('Run tests on different versions of vanilla K8s') {
            // Do not forget to keep in sync the kind node image versions in `.ci/packer_cache.sh`.
            parallel {
                stage("1.12.10") {
                    steps {
                        checkout scm
                        runTests("kindest/node:v1.12.10")
                    }
                }
                stage("1.16.4") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        checkout scm
                        runTests("kindest/node:v1.16.4")
                    }
                }
                stage("1.17.0") {
                    agent {
                        label 'eck'
                    }
                    steps {
                        checkout scm
                        runTests("kindest/node:v1.17.0")
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
                    def msg = testScript.generateSlackMessage("E2E tests for different versions of vanilla K8s failed!", env.BUILD_URL, filter)

                    slackSend botUser: true,
                        channel: '#cloud-k8s',
                        color: 'danger',
                        message: msg,
                        tokenCredentialId: 'cloud-ci-slack-integration-token'
                }
            }
        }
        cleanup {
            cleanWs()
        }
    }

}

def runTests(kindImage) {
    sh """
        cat >.env <<EOF
OPERATOR_IMAGE = ${IMAGE}
GCLOUD_PROJECT = $GCLOUD_PROJECT
SKIP_DOCKER_COMMAND = true
REGISTRY = eu.gcr.io
REPOSITORY = $GCLOUD_PROJECT
E2E_JSON = true
KIND_NODE_IMAGE = ${kindImage}
TEST_LICENSE = /go/src/github.com/elastic/cloud-on-k8s/.ci/test-license.json
GO_TAGS = release
export LICENSE_PUBKEY = /go/src/github.com/elastic/cloud-on-k8s/.ci/license.key
EOF
    """
    script {
        env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-license get-elastic-public-key TARGET=kind-e2e ci')

        sh 'make -C .ci TARGET=e2e-generate-xml ci'
        junit "e2e-tests.xml"

        if (env.SHELL_EXIT_CODE != 0) {
            failedTests.addAll(testScript.getListOfFailedTests())
        }

        sh 'exit $SHELL_EXIT_CODE'
    }
}
