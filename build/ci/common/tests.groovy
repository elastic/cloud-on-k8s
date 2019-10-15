// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// Common test related code for Jenkins pipelines

def getListOfFailedTests(details = "") {
    def failed = currentBuild.rawBuild.getAction(hudson.tasks.test.AbstractTestResultAction.class)?.getResult()?.getFailedTests()
    def result = []
    failed.each { ft ->
        def sb = new StringBuilder()
        if (details != "") {
            sb.append(String.format("Details: %s", details))
            sb.append("\r\n")
        }
        sb.append(String.format("%s\r\n%s", ft.getDisplayName(), ft.getErrorStackTrace()))
        result.add(sb.toString())
    }
    return result
}

return this
