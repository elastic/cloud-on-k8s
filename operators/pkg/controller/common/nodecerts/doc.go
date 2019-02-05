package nodecerts

//
// Final intended workflow
//
// 1. create placeholder secret for node cert + ca
// 2. pod creates CSR-like, pushes to api server
// 3. validate csr originates from pod (TODO: decide on how)
// 4. issue certificate based on csr, fill in placeholder secret
// 5. whenever our basis for the issued cert changes, update placeholder secret
//
// 1. create placeholder secret for node cert + ca
// 2. cant wait for a csr, so pretend we have one..
// 3. issue certificate based on csr
// 3. fill in placeholder secret with node cert + ca + private keys (ugh)
