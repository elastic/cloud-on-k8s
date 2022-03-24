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

    parameters {
        string(name: "OPERATOR_IMAGE", defaultValue: "", description: "ECK Docker image")
        string(name: "ELASTICSEARCH_IMAGE", defaultValue: "", description: "Elasticsearch image under test")
        string(name: "ELASTICSEARCH_VERSION", defaultValue: "", description: "Elasticsearch version under test")
        string(name: "KIBANA_IMAGE", defaultValue: "", description: "Kibana image under test")
        string(name: "KIBANA_VERSION", defaultValue: "", description: "Kibana version under test")
        string(name: "ELASTIC_AGENT_IMAGE", defaultValue: "", description: "Elastic Agent image under test")
        string(name: "ELASTIC_AGENT_VERSION", defaultValue: "", description: "Elastic Agent version under test")
        string(name: "FLEET_SERVER_IMAGE", defaultValue: "", description: "Fleet Server image under test")
        string(name: "FLEET_SERVER_VERSION", defaultValue: "", description: "Fleet Server version under test")
        string(name: "ENTERPRISE_SEARCH_IMAGE", defaultValue: "", description: "Enterprise Search image under test")
        string(name: "ENTERPRISE_SEARCH_VERSION", defaultValue: "", description: "Enterprise Search version under test")
        string(name: "ELASTIC_MAPS_SERVER_IMAGE", defaultValue: "", description: "Elastic Maps Server image under test")
        string(name: "ELASTIC_MAPS_SERVER_VERSION", defaultValue: "", description: "Elastic Maps Server version under test")
        // Beats images are not supported here we would need 12 additional inputs to accomplish that
        // APM Server is also not supported here given that is deprecated in favour of the Elastic Agent APM integration
        string(name: "TESTS_MATCH", defaultValue: "^Test", description: "Regular expression to select which test cases to run")
        string(name: "TEST_TAGS", defaultValue: "e2e", description: "Go build constraint to select a group of e2e tests. Supported values are: e2e, es, kb, beat, agent, ems, ent")
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
        JKS_PARAM_OPERATOR_IMAGE = "${params.OPERATOR_IMAGE}" // for bwc with setenvconfig
    }

    stages {
        stage('Load common scripts') {
            steps {
                script {
                    lib = load ".ci/common/tests.groovy"
                }
            }
        }
        stage("Run E2E tests") {
            steps {
                sh '.ci/setenvconfig e2e/custom'
                script {
                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-artifacts TARGET=ci-e2e ci')

                    sh 'make -C .ci TARGET=e2e-generate-xml ci'
                    junit "e2e-tests.xml"

                    if (env.SHELL_EXIT_CODE != 0) {
                        failedTests = lib.getListOfFailedTests()
                        googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                            credentialsId: "devops-ci-gcs-plugin",
                            pattern: "*.zip",
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
                if (params.SEND_NOTIFICATIONS) {
                    Set<String> filter = new HashSet<>()
                    filter.addAll(failedTests)
                    def msg = lib.generateSlackMessage("Custom E2E tests run failed!", env.BUILD_URL, filter)

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
             script {
               build job: 'cloud-on-k8s-e2e-cleanup',
                     parameters: [string(name: 'JKS_PARAM_GKE_CLUSTER', value: "eck-e2e-custom-${BUILD_NUMBER}")],
                     wait: false
                }

            cleanWs()
        }
    }
}
