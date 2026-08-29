FROM busybox:1.37.0
COPY guest/docker-proof.sh /docker-proof
RUN chmod 0755 /docker-proof
CMD ["/docker-proof"]
