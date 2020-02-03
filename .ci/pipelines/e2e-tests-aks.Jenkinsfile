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
        stage('Checkout from GitHub') {
            steps {
                checkout scm
            }
        }
        stage('Run Checks') {
            steps {
                sh 'make -C .ci TARGET=ci-check ci'
            }
        }
        stage("Run E2E tests") {
            steps {
                script {
                    sh '.ci/setenvconfig e2e/aks'
                    env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C .ci get-test-license get-elastic-public-key TARGET=ci-e2e ci')

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
                sh """
                    cat >deployer-config.yml <<EOF
id: aks-ci
overrides:
  operation: delete
  clusterName: $BUILD_TAG
  vaultInfo:
    address: $VAULT_ADDR
    roleId: $VAULT_ROLE_ID
    secretId: $VAULT_SECRET_ID
EOF
                    make -C .ci TARGET=run-deployer ci
                """
            }
            cleanWs()
        }
    }
}
