FROM alpine:3.6

ADD dockerPlexTranscoder /kube-plex

ENTRYPOINT kube-plex