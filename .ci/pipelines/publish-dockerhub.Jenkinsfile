// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

pipeline {

    agent {
        label 'linux'
    }

    options {
        timeout(time: 10, unit: 'MINUTES')
    }

    environment {
        VAULT_ADDR = credentials('vault-addr')
        VAULT_ROLE_ID = credentials('vault-role-id')
        VAULT_SECRET_ID = credentials('vault-secret-id')
    }

    parameters {
        string(name: "DRY_RUN", defaultValue: "true", description: "If true, image is published to docker.elastic.co/eck-dev, otherwise to docker.io")
        string(name: "ECK_VERSION", defaultValue: "", description: "ECK version to publish")
    }

    stages {
        stage("Publish ECK to Docker Hub") {
            steps {
                sh 'hack/publish-dockerhub.sh'
            }
        }
    }
}
