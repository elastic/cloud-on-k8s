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
        // read safely TAG_NAME, defined for a release build and not for a nightly build
        TAG_NAME = sh(script: 'echo -n $TAG_NAME', returnStdout: true)
    }

    stages {
        stage('Nightly or release build - Check License Header and Lint') {
            failFast true
            parallel {
                stage('Check License Header') {
                    steps {
                        sh 'make -C .ci TARGET=check-license-header ci'
                    }
                }
                stage('Lint') {
                    steps {
                        sh 'make -C .ci TARGET=lint ci'
                    }
                }
            }
        }
        stage('Nightly or release build - Shellcheck and Generation') {
            failFast true
            parallel {
                stage('Shellcheck') {
                    steps {
                        sh 'make -C .ci TARGET=shellcheck ci'
                    }
                }
                stage('Generate and check local changes') {
                    steps {
                        sh 'make -C .ci TARGET=generate-and-check-local-changes ci'
                    }
                }
            }
        }
        stage('Nightly or release build - Unit and Integration tests') {
            failFast true
            parallel {
                stage('Unit tests') {
                    steps {
                        sh 'make -C .ci TARGET=unit-xml ci'
                    }
                }
                stage('Integration tests') {
                    steps {
                        sh 'make -C .ci TARGET=integration-xml ci'
                    }
                }
                stage('Ensure reattach-pv compiles') {
                    steps {
                        sh 'make -C .ci TARGET=reattach-pv ci'
                    }
                }
            }
        }
        stage('Nightly or release build - Build and Push') {
            stages {
                stage('Build Docker image') {
                    steps {
                        sh 'make -C .ci TARGET=docker-build ci'
                    }
                }
                stage('Build and push multiarchitecture Docker image') {
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
                stage('Notify successful release build') {
                    when {
                        buildingTag()
                    }
                    steps {
                        script {

                            slackSend(
                                channel: '#cloud-k8s',
                                color: 'good',
                                message: "`${TAG_NAME}` was released \r\n" +
                                    "Manifests were uploaded to https://download.elastic.co/downloads/eck/${TAG_NAME}\r\n" +
                                    "Congratulations!",
                                tokenCredentialId: 'cloud-ci-slack-integration-token'
                            )
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
                if (isWeekday()) {
                    build job: 'cloud-on-k8s-e2e-tests-stack-versions',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    build job: 'cloud-on-k8s-e2e-tests-gke-k8s-versions',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    build job: 'cloud-on-k8s-e2e-tests-aks',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    build job: 'cloud-on-k8s-e2e-tests-kind-k8s-versions',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    // test the latest version of OCP on every build
                    build job: 'cloud-on-k8s-e2e-tests-ocp',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'OCP_VERSION', value: "4.9.0"),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    build job: 'cloud-on-k8s-e2e-tests-eks',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    build job: 'cloud-on-k8s-e2e-tests-eks-arm',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false

                    build job: 'cloud-on-k8s-e2e-tests-resilience',
                        parameters: [
                            string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: operatorImage),
                            string(name: 'branch_specifier', value: GIT_COMMIT)
                        ],
                        wait: false
                } else {

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

def isWeekday() {
     // %u day of week (1..7); 1 is Monday 5 is Friday
     int day = sh (
         script: "date +%u",
         returnStdout: true
     ) as Integer
     return day <= 5
 }
