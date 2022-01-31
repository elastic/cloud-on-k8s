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
        stage('Run checks in parallel') {
            failFast true
            parallel {
                stage('Validate Jenkins pipelines') {
                    when {
                        expression {
                            arePipelinesModified()
                        }
                    }
                    steps {
                        sh 'make -C .ci TARGET=validate-jenkins-pipelines ci'
                    }
                }
                stage('Run checks') {
                    when {
                        expression {
                            notOnlyDocs()
                        }
                    }
                    steps {
                        sh 'make -C .ci TARGET=ci-check ci'
                    }
                }
            }
        }
        stage('Run tests in parallel') {
            parallel {
                stage("Run unit and integration tests") {
                    when {
                        expression {
                            notOnlyDocs()
                        }
                    }
                    steps {
                        script {
                            env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci TARGET=ci ci')

                            junit "unit-tests.xml"
                            junit "integration-tests.xml"

                            sh 'exit $SHELL_EXIT_CODE'
                        }
                    }
                }
                stage("Run smoke E2E tests") {
                    when {
                        expression {
                            notOnlyDocs()
                        }
                    }
                    steps {
                        sh '.ci/setenvconfig pr'
                        script {
                            env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-artifacts TARGET=ci-build-operator-e2e-run ci')

                            sh 'make -C .ci TARGET=e2e-generate-xml ci'
                            junit "e2e-tests.xml"

                            sh 'exit $SHELL_EXIT_CODE'
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
