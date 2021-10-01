// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

pipeline {

    agent {
        label 'linux'
    }

    options {
        timeout(time: 1, unit: 'HOURS')
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
        // TAG_NAME must always be empty this is never a release build, we use this in setenvconfig to decided
        // between dev and release build
        TAG_NAME = ""
    }

    stages {
        stage('Extended e2e builds') {
            stages {
                stage('Run checks') {
                    steps {
                        sh 'make -C .ci TARGET=ci-check ci'
                    }
                }
                stage('Run unit and integration tests') {
                    steps {
                        sh 'make -C .ci TARGET=ci ci'
                    }
                }
                stage('Build and push Docker image') {
                    steps {
                        sh '.ci/setenvconfig build'
                        sh 'make -C .ci license.key TARGET=ci-release ci'
                    }
                }
                stage('Upload YAML manifest to S3') {
                    environment {
                        VERSION="${sh(returnStdout: true, script: '. ./.env; echo $IMG_VERSION').trim()}"
                    }

                    steps {
                        script {
                            sh 'make -C .ci yaml-upload'
                        }
                    }
                }
            }
        }
    }

    post {
        success {
            script {
                def operatorImage = sh(returnStdout: true, script: 'make print-operator-image').trim()


                build job: 'cloud-on-k8s-e2e-tests-ocp-all-but-latest',
                    parameters: [
                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                        string(name: 'branch_specifier', value: GIT_COMMIT)
                    ],
                    wait: false

                build job: 'cloud-on-k8s-e2e-tests-tanzu',
                    parameters: [
                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                        string(name: 'branch_specifier', value: GIT_COMMIT)
                    ],
                    wait: false


            }
        }
        unsuccessful {
            script {
                slackSend channel: '#cloud-k8s',
                    color: 'danger',
                    message: "${JOB_NAME} job failed! \r\n" + "${BUILD_URL}",
                    tokenCredentialId: 'cloud-ci-slack-integration-token'
            }
        }
        cleanup {
            cleanWs()
        }
    }
}
