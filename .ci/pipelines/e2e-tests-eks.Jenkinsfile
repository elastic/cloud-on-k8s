// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

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
    }

    stages {
        stage("Run E2E tests") {
            steps {
                sh '.ci/setenvconfig e2e/eks'
                script {
                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-artifacts TARGET=ci-e2e ci')

                    sh 'make -C .ci TARGET=e2e-generate-xml ci'
                    junit "e2e-tests.xml"

                    if (env.SHELL_EXIT_CODE != 0) {
                        googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                            credentialsId: "devops-ci-gcs-plugin",
                            pattern: "*.tgz",
                            sharedPublicly: true,
                            showInline: true
                    }

                    sh 'exit $SHELL_EXIT_CODE'
                }
            }
        }
    }

    post {
        cleanup {
            script {
                sh '.ci/setenvconfig cleanup/eks'
                sh 'make -C .ci TARGET=run-deployer ci'
            }
            cleanWs()
        }
    }
}
