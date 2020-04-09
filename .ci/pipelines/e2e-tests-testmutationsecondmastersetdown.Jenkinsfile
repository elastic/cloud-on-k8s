// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

pipeline {

    agent {
        label 'linux'
    }

    options {
        timeout(time: 24, unit: 'HOURS')
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
    }

    stages {
        stage('Checkout from GitHub') {
            steps {
                checkout scm
            }
        }
        stage("E2E tests") {
            steps {
                sh '.ci/setenvconfig e2e/master'

                sh 'echo TESTS_MATCH = TestMutationSecondMasterSetDown >> .env'
                sh 'echo E2E_SKIP_CLEANUP = true >> .env'

                sh 'make -C .ci get-test-artifacts TARGET=ci-build-operator-e2e-run-indefinitely ci'
            }
        }
    }

    post {
        cleanup {
            cleanWs()
        }
        // K8s cluster will not be cleanup.
    }
}
