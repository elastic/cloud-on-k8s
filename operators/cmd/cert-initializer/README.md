# Cert initializer

Intended to run as an init container in an Elasticsearch pod, it handles private key and CSR generation in order for the operator to deliver a TLS certificate.

## Flow of operations

```text
                                 +-----------------------------+
                                 |          ES Pod             |
                                 | +------------------+        |
+-------------+                  | |    Cert init     |        |
|             |       HTTP       | |                  |        |
|  Operator   +--------------------> csr              |        |
|             |                  | |                  |        |
+-------------+                  | |                  |        |
         |                       | +--      pkey     -+        |
         |                       +----  cert + csr   ----------+
         |                                 ^
         +---------------------------------+
                     secret mount
```

1. Operator creates a pod
2. Cert-initializer is started in an init container
3. If valid private key, cert and CSR already exist in the pod, we're done
4. Create a private key and a CSR
5. Store the private key in a volume persisted through pod restarts
6. Serve the CSR via an HTTP server (`GET /csr`)
7. Operator requests the CSR
8. Operator creates a signed cert from the CSR, and mounts it through a secret volume (with the CSR)
9. Cert-initializer monitors the mounted certificate file: once correct, we're done

## Expected shared volumes

* Private key: created and read by the cert-initializer, shared with the Elasticsearch container once started. Must persist across pod restarts through a volume (emptyDir should be fine).
* CSR file: mounted in a secret volume by the operator, read by the cert-initializer.
* Cert file: mounted in a secret volume by the operator, read by the cert-initializer and the Elasticsearch container.

## Reusing data

Since the operator stores the CSR retrieved from the cert-initializer into a secret, it is able to issue a new certificate compatible with a previous CSR (and with an existing private key in the ES pod). This allows rotation of the CA cert.
If the ES pod gets restarted, the private key may be preserved if mounted on a persistent volume. In such case, the existing certificate can be reused.
For cases where the private key is not preserved, a new one will be generated, incompatible with the existing certificate. The operator will notice the cert-initializer init container is still in a "Running" state, hence request for the new CSR corresponding to the new private key, and update the certificate accordingly.
