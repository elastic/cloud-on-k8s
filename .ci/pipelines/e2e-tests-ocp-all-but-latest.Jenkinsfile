// This library overrides the default checkout behavior to enable sleep+retries if there are errors
// Added to help overcome some recurring github connection issues
@Library('apm@current') _

def failedTests = []
def lib

pipeline {

    agent {
        label 'linux'
    }

    options {
        timeout(time: 50, unit: 'HOURS')
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
                    lib = load ".ci/common/tests.groovy"
                }
            }
        }
        stage("Run E2E tests") {
            when {
               expression {
                    // this is a downstream job but let's not run it every day due to the amount of resources
                    // it requires
                    isFriday()
               }
            }
            steps {
                // "4.3.40", "4.4.33", "4.5.37", "4.6.24", "4.7.6"
                // latest 4.8.x is taken care of by a separate job
                build job: 'cloud-on-k8s-e2e-tests-ocp',
                                    parameters: [
                                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: JKS_PARAM_OPERATOR_IMAGE),
                                        string(name: 'OCP_VERSION', value: "4.3.40"),
                                        string(name: 'branch_specifier', value: GIT_COMMIT)
                                    ],
                                    wait: true
                build job: 'cloud-on-k8s-e2e-tests-ocp',
                                    parameters: [
                                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: JKS_PARAM_OPERATOR_IMAGE),
                                        string(name: 'OCP_VERSION', value: "4.4.33"),
                                        string(name: 'branch_specifier', value: GIT_COMMIT)
                                    ],
                                    wait: true
                build job: 'cloud-on-k8s-e2e-tests-ocp',
                                    parameters: [
                                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: JKS_PARAM_OPERATOR_IMAGE),
                                        string(name: 'OCP_VERSION', value: "4.5.37"),
                                        string(name: 'branch_specifier', value: GIT_COMMIT)
                                    ],
                                    wait: true
                build job: 'cloud-on-k8s-e2e-tests-ocp',
                                    parameters: [
                                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: JKS_PARAM_OPERATOR_IMAGE),
                                        string(name: 'OCP_VERSION', value: "4.6.24"),
                                        string(name: 'branch_specifier', value: GIT_COMMIT)
                                    ],
                                    wait: true
                build job: 'cloud-on-k8s-e2e-tests-ocp',
                                    parameters: [
                                        string(name: 'JKS_PARAM_OPERATOR_IMAGE', value: JKS_PARAM_OPERATOR_IMAGE),
                                        string(name: 'OCP_VERSION', value: "4.7.6"),
                                        string(name: 'branch_specifier', value: GIT_COMMIT)
                                    ],
                                    wait: true
            }
        }
    }
}

def isFriday() {
    // %u day of week (1..7); 1 is Monday 5 is Friday
    return sh (
        script: "date +%u",
        returnStdout: true
    ) as Integer == 5
}
