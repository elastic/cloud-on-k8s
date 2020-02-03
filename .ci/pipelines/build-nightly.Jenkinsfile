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
        REGISTRY = "docker.elastic.co"
        REPOSITORY = "eck-snapshots"
        IMG_NAME = "eck-operator"
    }

    stages {
        stage('Run checks') {
            steps {
                sh 'make -C .ci TARGET=ci-check ci'
            }
        }
        stage('Run unit and integration tests') {
            steps {
                sh """
                    make -C .ci TARGET=ci ci
                """
            }
        }
        stage('Build and push Docker image') {
            steps {
                sh """
                    echo \$OPERATOR_IMAGE > eck_image.txt
                    make -C .ci get-docker-creds get-elastic-public-key TARGET=ci-release ci
                """
            }
        }
    }

    post {
        success {
            script {
                def image = readFile("$WORKSPACE/eck_image.txt").trim()
                currentBuild.description = image

                build job: 'cloud-on-k8s-versions-gke',
                      parameters: [string(name: 'IMAGE', value: image)],
                      wait: false

                build job: 'cloud-on-k8s-stack',
                      parameters: [string(name: 'IMAGE', value: image)],
                      wait: false
            }
        }
        cleanup {
            cleanWs()
        }
    }
}
