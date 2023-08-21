FROM alpine:latest

ARG BUILDARCH
# copy file to tmp dir
COPY target/pocketbase_go_linux /pb/pocketbase

RUN apk add --no-cache \
    unzip \
    ca-certificates



# uncomment to copy the local pb_migrations dir into the image
# COPY ./pb_migrations /pb/pb_migrations

# uncomment to copy the local pb_hooks dir into the image
# COPY ./pb_hooks /pb/pb_hooks
EXPOSE 8090

# start PocketBase
CMD ["/pb/pocketbase", "serve", "--http=0.0.0.0:8090"]