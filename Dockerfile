FROM quay.io/gravitational/debian-tall:jessie
MAINTAINER Grvitational Inc <admin@gravitational.com>

ADD build/satellite /usr/local/bin/
ADD build/healthz /usr/local/bin/

EXPOSE 7575 8080

