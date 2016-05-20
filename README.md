# Unfollowers

This is a tool for monitoring Twitter followers and unfollowers,
including accounts that have been deactivated.

Note that you need to install go-bindata and run `go generate` before
doing `go build`.

It runs a web server and launches a web browser instead of attempting to
create a cross platform GUI from Go. It stores all data locally using
a SQLite database.
