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
        REPOSITORY = "eck"
        IMG_NAME = "eck-operator"
        REGISTRY = "docker.elastic.co"
        OPERATOR_IMAGE = "${REGISTRY}/${REPOSITORY}/${IMG_NAME}:${TAG_NAME}"
    }

    stages {
        stage('Doing release') {
            when {
                expression {
                    releaseImageNotExist()
                }
            }
            stages {
                stage('Build and push release image') {
                    steps {
                        sh """
                            make -C .ci get-docker-creds get-elastic-public-key TARGET=ci-release ci
                        """
                    }
                }
                stage('Upload yaml to S3') {
                    steps {
                        sh 'make -C .ci yaml-upload'
                    }
                }
                stage('Send message to Slack') {
                    steps {
                        script {
                            def msg = "${OPERATOR_IMAGE} was pushed \r\n" +
                                "https://download.elastic.co/downloads/eck/${TAG_NAME}/all-in-one.yaml was uploaded \r\n" +
                                "Congratulations!"
                            slackSend botUser: true,
                                channel: '#cloud-k8s',
                                color: 'good',
                                message: msg,
                                tokenCredentialId: 'cloud-ci-slack-integration-token'
                        }
                    }
                }
            }
        }
    }

    post {
        success {
            build job: 'cloud-on-k8s-e2e-tests-custom',
                parameters: [string(name: 'IMAGE', value: "${OPERATOR_IMAGE}")],
                wait: false

            build job: 'cloud-on-k8s-stack',
                parameters: [string(name: 'IMAGE', value: "${OPERATOR_IMAGE}")],
                wait: false
                
            build job: 'cloud-on-k8s-versions-gke',
                parameters: [string(name: 'IMAGE', value: "${OPERATOR_IMAGE}")],
                wait: false
        }
        unsuccessful {
            script {
                def msg = "Release job failed! \r\n" +
                          "${BUILD_URL}"
                slackSend botUser: true,
                    channel: '#cloud-k8s',
                    color: 'danger',
                    message: msg,
                    tokenCredentialId: 'cloud-ci-slack-integration-token'
            }
        }
        cleanup {
            cleanWs()
        }
    }

}

def releaseImageNotExist() {
    return sh (
        script: "docker pull $OPERATOR_IMAGE",
        returnStatus: true
    ) == 1
}
