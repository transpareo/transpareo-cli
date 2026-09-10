// Package signer is the reference implementation of the signing
// endpoint a workspace runs when it brings its own keys. The
// platform POSTs what is to be signed at publish time and reads
// the signatures back; the keys never leave the workspace.
//
// Two request shapes arrive on one route. Whole-document
// signatures (eddsa-jcs-2022) take a canonical body and proof
// configurations and answer one Ed25519 proof value per
// configuration. A base proof (ecdsa-sd-2023) takes the proof
// and mandatory hashes and the non-mandatory statements of a
// passport and answers the components the platform assembles
// into a selective-disclosure proof, signed with a P-256 key
// generated for that one proof and the workspace's P-256 key.
//
// Every request is signed by the platform host that serves the
// workspace; Verifier checks that signature, the timestamp and
// the nonce before Signer touches a key. Handler wires both
// into an http.Handler; the command-line tool serves it as
// `transpareo signer serve`.
package signer
