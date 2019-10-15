// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Common test related code for Jenkins pipelines

def getListOfFailedTests(details = "") {
    def failed = currentBuild.rawBuild.getAction(hudson.tasks.test.AbstractTestResultAction.class)?.getResult()?.getFailedTests()
    def result = []
    failed.each { ft ->
        result.add(String.format("%s\r\n%s", ft.getDisplayName(), ft.getErrorStackTrace()))
    }
    return result
}

return this
