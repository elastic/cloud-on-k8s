#!/usr/bin/env python

import os


def get_env_var(var):
    try:
        return os.environ[var]
    except:
        return ""


def get_value_from_file(file):
    try:
        tf = open(file, "r")
        return tf.read()
    except:
        return ""


def write_env_to_file(key, file):
    var = get_env_var(key)
    if len(var) > 0:
        file.write("{}={}\r\n".format(key.upper(), var))


with open("environment", "w") as text_file:
    docker_login = get_value_from_file("docker_login.file")
    if docker_login != "":
        text_file.write("{}={}\r\n".format("ELASTIC_DOCKER_LOGIN", docker_login))

    docker_password = get_value_from_file("docker_credentials.file")
    if docker_password != "":
        text_file.write("{}={}\r\n".format("ELASTIC_DOCKER_PASSWORD", docker_password))

    aws_access = get_value_from_file("aws_access_key.file")
    if aws_access != "":
        text_file.write("{}={}\r\n".format("AWS_ACCESS_KEY_ID", aws_access))

    aws_secret = get_value_from_file("aws_secret_key.file")
    if aws_secret != "":
        text_file.write("{}={}\r\n".format("AWS_SECRET_ACCESS_KEY", aws_secret))

    gcp_creds = get_value_from_file("credentials.json")
    if gcp_creds != "":
        text_file.write("{}={}\r\n".format("GKE_SERVICE_ACCOUNT_KEY_FILE",
        "/go/src/github.com/elastic/cloud-on-k8s/build/ci/credentials.json"))

    write_env_to_file("OPERATOR_IMAGE", text_file)
    write_env_to_file("LATEST_RELEASED_IMG", text_file)
    write_env_to_file("VERSION", text_file)
    write_env_to_file("SNAPSHOT", text_file)
    write_env_to_file("GCLOUD_PROJECT", text_file)
    write_env_to_file("REGISTRY", text_file)
    write_env_to_file("REPOSITORY", text_file)
    write_env_to_file("IMG_SUFFIX", text_file)
    write_env_to_file("GKE_CLUSTER_NAME", text_file)
    write_env_to_file("TESTS_MATCH", text_file)
    write_env_to_file("GKE_CLUSTER_VERSION", text_file)




