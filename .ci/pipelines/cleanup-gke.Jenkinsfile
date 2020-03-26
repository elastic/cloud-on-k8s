// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

pipeline {

    agent {
        label 'linux'
    }

    options {
        timeout(time: 30, unit: 'MINUTES')
        retry(3)
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
    }

    stages {
        stage('Cleanup GKE') {
            options {
                retry(3)
            }
            steps {
                sh '.ci/setenvconfig cleanup/gke'
                sh 'make -C .ci TARGET=run-deployer ci'
            }
        }
    }

    post {
        cleanup {
            cleanWs()
        }
    }

}
