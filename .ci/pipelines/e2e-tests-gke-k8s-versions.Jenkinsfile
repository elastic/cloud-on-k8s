def failedTests = []
def lib

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
        GCLOUD_PROJECT = credentials('k8s-operators-gcloud-project')
    }

    stages {
        stage('Load common scripts') {
            steps {
                script {
                    lib = load "build/ci/common/tests.groovy"
                }
            }
        }
        stage('Run tests for different k8s versions in GKE') {
            parallel {
                stage("1.13") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, '1.13', "eck-gke13-${BUILD_NUMBER}-e2e")
                        }
                    }
                }
                stage("1.14") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, '1.14', "eck-gke14-${BUILD_NUMBER}-e2e")
                        }
                    }
                }
                stage("1.15") {
                    agent {
                        label 'linux'
                    }
                    steps {
                        checkout scm
                        script {
                            runWith(lib, failedTests, '1.15', "eck-gke15-${BUILD_NUMBER}-e2e")
                        }
                    }
                }
            }
        }
    }

    post {
        unsuccessful {
            script {
                if (params.SEND_NOTIFICATIONS) {
                    Set<String> filter = new HashSet<>()
                    filter.addAll(failedTests)
                    def msg = lib.generateSlackMessage("E2E tests for different k8s versions in GKE failed!", env.BUILD_URL, filter)

                    slackSend botUser: true,
                        channel: '#cloud-k8s',
                        color: 'danger',
                        message: msg,
                        tokenCredentialId: 'cloud-ci-slack-integration-token'
                }
            }
        }
        cleanup {
            script {
                clusters = ["eck-gke13-${BUILD_NUMBER}-e2e", "eck-gke14-${BUILD_NUMBER}-e2e", "eck-gke15-${BUILD_NUMBER}-e2e"]
                for (int i = 0; i < clusters.size(); i++) {
                    build job: 'cloud-on-k8s-e2e-cleanup',
                        parameters: [string(name: 'GKE_CLUSTER', value: clusters[i])],
                        wait: false
                }
            }
            cleanWs()
        }
    }
}

def runWith(lib, failedTests, clusterVersion, clusterName) {
    sh """#!/bin/bash

        cat >.env <<EOF
GCLOUD_PROJECT = $GCLOUD_PROJECT
OPERATOR_IMAGE = $IMAGE
REGISTRY = eu.gcr.io
REPOSITORY = $GCLOUD_PROJECT
SKIP_DOCKER_COMMAND = true
E2E_JSON = true
TEST_LICENSE = /go/src/github.com/elastic/cloud-on-k8s/build/ci/test-license.json
GO_TAGS = release
export LICENSE_PUBKEY = /go/src/github.com/elastic/cloud-on-k8s/build/ci/license.key
EOF
    cat >deployer-config.yml <<EOF
id: gke-ci
overrides:
  operation: create
  kubernetesVersion: "${clusterVersion}"
  clusterName: ${clusterName}
  vaultInfo:
    address: $VAULT_ADDR
    roleId: $VAULT_ROLE_ID
    secretId: $VAULT_SECRET_ID
  gke:
    gCloudProject: $GCLOUD_PROJECT
EOF
    """
    script {
        env.SHELL_EXIT_CODE = sh(returnStatus: true, script: 'make -C build/ci get-test-license get-elastic-public-key TARGET=ci-e2e ci')

        sh 'make -C build/ci TARGET=e2e-generate-xml ci'
        junit "e2e-tests.xml"

        if (env.SHELL_EXIT_CODE != 0) {
            failedTests.addAll(lib.getListOfFailedTests())
            googleStorageUpload bucket: "gs://devops-ci-artifacts/jobs/$JOB_NAME/$BUILD_NUMBER",
                credentialsId: "devops-ci-gcs-plugin",
                pattern: "*.tgz",
                sharedPublicly: true,
                showInline: true
        }

        sh 'exit $SHELL_EXIT_CODE'
    }
}
