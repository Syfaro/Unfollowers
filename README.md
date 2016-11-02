# Unfollowers

This is a tool for monitoring Twitter followers and unfollowers,
including accounts that have been deactivated.

Note that you need to install go-bindata and run `go generate` before
doing `go build`.  See [Installation](#installation) for details.

It runs a web server and launches a web browser instead of attempting to
create a cross platform GUI from Go. It stores all data locally using
a SQLite database.

## Installation

~~~sh
# set GOPATH and PATH (if you haven't already)
export GOPATH="$HOME/.local" # or any other choice
export PATH="$GOPATH/bin:$PATH"

# install go-bindata (if you haven't already)
go get github.com/jteeuwen/go-bindata/go-bindata

# install unfollowers
go get -d github.com/Syfaro/unfollowers
go generate github.com/Syfaro/unfollowers
go install github.com/Syfaro/unfollowers
~~~
