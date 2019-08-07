#!/usr/bin/env python

import os


def get_env_var(var):
    try:
        return os.environ[var]
    except:
        return ""


def get_value_from_file(file):
    try:
        with open(file, "r") as tf:
            return tf.read()
    except:
        return ""


def write_to_file(key, value, file):
    if value != "":
        file.write("{}={}\r\n".format(key, value))


def file_read_env_and_write(var, filename, file_to_write):
    write_to_file(var, get_value_from_file(filename), file_to_write)


def write_env_to_file(key, file):
    write_to_file(key, get_env_var(key), file)


with open("environment", "w") as text_file:
    gcp_creds = get_value_from_file("credentials.json")
    if gcp_creds != "":
        text_file.write("{}={}\r\n".format("GKE_SERVICE_ACCOUNT_KEY_FILE",
        "/go/src/github.com/elastic/cloud-on-k8s/build/ci/credentials.json"))

    file_read_env_and_write("ELASTIC_DOCKER_LOGIN", "docker_login.file", text_file)
    file_read_env_and_write("ELASTIC_DOCKER_PASSWORD", "docker_credentials.file", text_file)
    file_read_env_and_write("AWS_ACCESS_KEY_ID", "aws_access_key.file", text_file)
    file_read_env_and_write("AWS_SECRET_ACCESS_KEY", "aws_secret_key.file", text_file)
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
    write_env_to_file("STACK_VERSION", text_file)
    write_env_to_file("SKIP_DOCKER_COMMAND", text_file)
