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
        stage('Run tests for different ELK stack versions in GKE') {
            parallel {
                stage("6.8.5") {
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-68-${BUILD_NUMBER}-e2e", "6.8.5")
                        }
                    }
                }
                stage("7.1.1") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-71-${BUILD_NUMBER}-e2e", "7.1.1")
                        }
                    }
                }
                stage("7.2.1") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-72-${BUILD_NUMBER}-e2e", "7.2.1")
                        }
                    }
                }
                stage("7.3.2") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-73-${BUILD_NUMBER}-e2e", "7.3.2")
                        }
                    }
                }
                stage("7.4.2") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-74-${BUILD_NUMBER}-e2e", "7.4.2")
                        }
                    }
                }
                stage("7.5.2") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-75-${BUILD_NUMBER}-e2e", "7.5.2")
                        }
                    }
                }
                stage("7.6.0") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, "eck-76-${BUILD_NUMBER}-e2e", "7.6.0")
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
                    "eck-68-${BUILD_NUMBER}-e2e",
                    "eck-71-${BUILD_NUMBER}-e2e",
                    "eck-72-${BUILD_NUMBER}-e2e",
                    "eck-73-${BUILD_NUMBER}-e2e",
                    "eck-74-${BUILD_NUMBER}-e2e",
                    "eck-75-${BUILD_NUMBER}-e2e",
                    "eck-76-${BUILD_NUMBER}-e2e"
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
