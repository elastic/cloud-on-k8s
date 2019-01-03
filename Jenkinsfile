#!/usr/bin/env groovy

node('swarm') {
    stage('Checkout from GitHub') {
	    checkout scm
    }
    stage("Make ci") {
        sh 'make ci'
    }
}
