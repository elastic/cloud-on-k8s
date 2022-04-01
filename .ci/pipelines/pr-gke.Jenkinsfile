// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

pipeline {

    agent {
        label 'eck'
    }

    options {
        timeout(time: 60, unit: 'MINUTES')
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
    }

    stages {
        stage('setup-ci-env') {
            when {
                expression {
                    notOnlyDocs()
                }
            }
            parallel {
                stage('build-image') {
                    steps {
                        sh 'make -C .ci get-test-artifacts ci-build-image'
                        sh '.ci/setenvconfig pr'
                    }
                }
                stage("validate-jenkins-pipelines") {
                    steps {
                        script {
                            sh 'make -C .ci TARGET=validate-jenkins-pipelines ci'
                        }
                    }
                }
            }
        }
        stage('tests') {
            when {
                expression {
                    notOnlyDocs()
                }
            }
            failFast true
            parallel {
                stage('tests') {
                    stages {
                        stage('lint') {
                            steps {
                                sh 'make -C .ci TARGET="lint check-local-changes" ci'
                            }
                        }
                        stage('generate') {
                            steps {
                                sh 'make -C .ci TARGET="generate check-local-changes" ci'
                            }
                        }
                        stage('check-license-header') {
                            steps {
                                sh 'make -C .ci TARGET=check-license-header ci'
                            }
                        }
                        stage('check-predicates') {
                            steps {
                                sh 'make -C .ci TARGET=check-predicates ci'
                            }
                        }
                        stage('shellcheck') {
                            steps {
                                sh 'make -C .ci TARGET=shellcheck ci'
                            }
                        }
                        stage("reattach-pv-tool") {
                            steps {
                                sh 'make -C .ci TARGET=reattach-pv ci'
                            }
                        }
                        stage("unit-tests") {
                            steps {
                                script {
                                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci TARGET=unit-xml ci')
                                    junit "unit-tests.xml"
                                    sh 'exit $SHELL_EXIT_CODE'
                                }
                            }
                        }
                        stage("integration-tests") {
                            steps {
                                script {
                                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci TARGET=integration-xml ci')
                                    junit "integration-tests.xml"
                                    sh 'exit $SHELL_EXIT_CODE'
                                }
                            }
                        }
                    }
                }
                stage('e2e-tests') {
                    stages {
                        stage("build-e2e-tests-docker-image") {
                            steps {
                                sh 'make -C .ci TARGET=e2e-docker-multiarch-build ci'
                            }
                        }
                        stage("build-operator-docker-image") {
                            steps {
                                sh 'make -C .ci TARGET=build-operator-image ci'
                            }
                        }
                        stage("create-k8s-cluster") {
                            steps {
                                sh 'make -C .ci TARGET="run-deployer apply-psp" ci'
                            }
                        }
                        stage('e2e-tests') {
                            steps {
                                script {
                                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci TARGET="set-kubeconfig e2e-run" ci')
                                    sh 'make -C .ci TARGET=e2e-generate-xml ci'
                                    junit "e2e-tests.xml"
                                    sh 'exit $SHELL_EXIT_CODE'
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    post {
        always {
            script {
                if (notOnlyDocs()) {
                    googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                        credentialsId: "devops-ci-gcs-plugin",
                        pattern: "*.zip",
                        sharedPublicly: true,
                        showInline: true
                }
            }
        }
        cleanup {
            script {
                if (notOnlyDocs()) {
                    build job: 'cloud-on-k8s-e2e-cleanup',
                        parameters: [string(name: 'JKS_PARAM_GKE_CLUSTER', value: "eck-pr-${BUILD_NUMBER}")],
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

def arePipelinesModified() {
    // grep succeeds if there is at least one line with .ci/jobs or .ci/pipelines
    return sh (
        script: "git diff --name-status HEAD~1 HEAD | grep -E '(.ci/jobs|.ci/pipelines)'",
        returnStatus: true
    ) == 0
}
