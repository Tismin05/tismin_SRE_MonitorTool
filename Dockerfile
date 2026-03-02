FROM ubuntu:latest
LABEL authors="tismin"

ENTRYPOINT ["top", "-b"]