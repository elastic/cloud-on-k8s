// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Common test related code for Jenkins pipelines

def getListOfFailedTests() {
    def failed = currentBuild.rawBuild.getAction(hudson.tasks.test.AbstractTestResultAction.class)?.getResult()?.getFailedTests()
    def result = []
    failed.each { ft ->
        result.add(ft.getDisplayName())
    }
    return result
}

def generateSlackMessage(baseMsg, URL, failedTests) {
    def sb = new StringBuilder()
    sb.append(baseMsg)
    sb.append("\r\n")
    sb.append(URL)
    sb.append("/testReport")
    if (failedTests.size() > 0) {
        sb.append("\r\n")
        sb.append("List of failed tests:")
        failedTests.each { ft ->
            sb.append("\r\n")
            sb.append(ft)
        }
    }
    return sb.toString()
}

// extractValueByKey extracts the value corresponding to a key in a string following the pattern: '.*key1=value1 key2=value2.*'.
// If there is no key/value pair matching the key, the provided default value is returned.
def extractValueByKey(str, key, defaultValue) {
    if (str != null) {
        def items = str.split(" ")
        for (int i = 0; i < items.length; i++) {
            def kv = items[i].split("=")
            if (kv.length == 2) {
                if (kv[0] == key) {
                    return kv[1]
                }
            }
        }
    }
    return defaultValue
}

return this
