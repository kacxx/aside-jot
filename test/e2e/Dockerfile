# Local clean-room: runs the e2e flow (test/e2e/run.sh) as a brand-new,
# unprivileged user with an empty HOME and no prior aside state, installing the
# published @latest exactly as a first-time user would.
#
#   docker build -t aside-e2e -f test/e2e/Dockerfile .
#   docker run --rm aside-e2e
#
# CI runs run.sh directly on the runner (no Docker); this image is for a clean
# local reproduction.
#
# Behind a TLS-intercepting corporate proxy, in-container HTTPS (the Go module
# proxy) will fail to verify certificates. Trust your proxy CA at build time,
# e.g. with an override that appends it before `go install`:
#   RUN cat your-proxy-ca.pem >> /etc/ssl/certs/ca-certificates.crt
# Keep that out of this file and out of git; it is environment-specific.
FROM golang:1.26-bookworm

RUN useradd -m tester
USER tester
WORKDIR /home/tester

COPY --chown=tester:tester run.sh /home/tester/run.sh
ENTRYPOINT ["bash", "/home/tester/run.sh"]
