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
                    lib = load ".ci/common/tests.groovy"
                }
            }
        }
        stage('Validate Jenkins pipelines') {
            steps {
                sh 'make -C .ci TARGET=validate-jenkins-pipelines ci'
            }
        }
        stage('Run checks') {
            steps {
                sh 'make -C .ci TARGET=ci-check ci'
            }
        }
         stage("Build dev operator image") {
            steps {
                sh '.ci/setenvconfig dev/build'
                sh('make -C .ci  get-docker-creds get-elastic-public-key TARGET=ci-release ci')
            }
         }
        stage('Run tests for different stack versions in GKE') {
            environment {
               // use the image we just built
               OPERATOR_IMAGE = """${sh(
                returnStdout: true,
                script: 'make print-operator-image'
                )}"""
            }
            parallel {
                stage("7.6.2-SNAPSHOT") {
                     agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-76-snapshot-${BUILD_NUMBER}-e2e", "7.6.1-SNAPSHOT")
                        }
                    }
                }
                stage("7.7.0-SNAPSHOT") {
                     agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-77-snapshot-${BUILD_NUMBER}-e2e", "7.7.0-SNAPSHOT")
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
                        message: lib.generateSlackMessage("E2E tests for Elastic stack snapshot versions failed!", env.BUILD_URL, filter),
                        tokenCredentialId: 'cloud-ci-slack-integration-token',
                        botUser: true,
                        failOnError: true
                    )
                }
                googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                    credentialsId: "devops-ci-gcs-plugin",
                    pattern: "*.tgz",
                    sharedPublicly: true,
                    showInline: true
            }
        }
        cleanup {
            script {
                clusters = [
                    "eck-76-snapshot-${BUILD_NUMBER}-e2e",
                    "eck-77-snapshot-${BUILD_NUMBER}-e2e"
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
    sh ".ci/setenvconfig e2e/stack-versions $clusterName $stackVersion"
    script {
        env.SHELL_EXIT_CODE = sh(returnStatus: true, script: "make -C .ci get-test-artifacts TARGET=ci-e2e ci")

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
