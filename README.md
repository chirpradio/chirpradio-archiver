# chirpradio-archiver

This is a long running program that listens to the live
[CHIRP Radio](http://chirpradio.org/) broadcast
stream and saves archive files to disk, one file per hour. It is the successor
to the archiving scripts in
[chirpradio-machine](https://github.com/chirpradio/chirpradio-machine/).

**IMPORTANT**: This is intended for internal station use only where we connect
directly to our streaming appliance. Please don't archive the live broadcast
yourself (unless testing) because CHIRP Radio pays per listener.

## Usage

Install [golang](http://golang.org/) (>= 1.14) and make sure your `$GOPATH` is
set. Put something like this in your shell profile:

    export GOPATH=$HOME/golang

Also make sure all Go executables are on your path:

    export PATH=$PATH:$GOPATH/bin

Install (or upgrade) the archiver on a production system:

    go get -u github.com/chirpradio/chirpradio-archiver

Now you can check out the options for the archiver:

    chirpradio-archiver -help

Here's an example of starting the archiver process:

    chirpradio-archiver -url=http://chirpradio.org/stream -dest=/path/to/archives

This will write archive files like the following example:

    /path/to/archives/2015/09/chirpradio_2015-09-21_000000.mp3

Any non-existing timestamp directories will be created.

## Deployment

As a CHIRP admin, follow these instructions to deploy the archiver:

1. On the `musiclib` server, switch to `archiver` and install the latest code from `master`:

```
sudo su archiver
go get -u github.com/chirpradio/chirpradio-archiver
```

2. From your own workstation (not the server), tag the revision that was released. For example:

```
git checkout master
git tag release-2020-07-16
git push --tags
```

3. Back on the server, restart the service:

```
sudo systemctl restart chirpradio-archiver
```

4. Check the log to see if it started OK:

```
sudo cat /home/archiver/log/archiver.log
```

5. Take a peek to see that archives are being written:

```
sudo du -sh /mnt/disk_array/archives/$(date +'%Y')/$(date +'%m')/$(date +'%d')/*
```

## Troubleshooting

The [systemd](https://www.freedesktop.org/wiki/Software/systemd/) script (in `/etc/systemd/system/chirpradio-archiver.service`) configures how the
`chirpradio-archiver` command is executed.
The service starts when the machine boots.

You can check `/home/archiver/log/archiver.log` to see its output or check
`/var/log/syslog` if there appears to be a more fatal error.

The systemd service is configured to restart on errors but restarting will stop if too many errors are encountered.

To restart the service manually, type:

```
sudo systemctl restart chirpradio-archiver
```

## Precautions

The archiver is deployed to a dedicated Linux server in CHIRP's studio so that
it can connect directly to the streaming appliance, as opposed to making an
external Internet connection to the relayed stream.

Here is an example of how the archiver runs:

    chirpradio-archiver \
        -url=http://192.X.X.X:8000/ \
        -dest=/mnt/disk_array/archives/ \
        -quiet >> $LOGFILE 2>&1

The `-url` in this case is a special streaming server that the broadcasting
applicance is configured to run within CHIRP's internal network.

**IMPORTANT**: This internal streaming server is only designed to handle one listener
which means you could knock down an archiver if you tried to connect to it
twice (e.g. for testing).

The `-dest` in this case points to the RAID array where archive files are
stored.

## Architecture

This program utilizes goroutines for robust and precise archiving. Here are some
notes:

1. A dedicated stream channel reads audio continously into a buffer so that no
   audio is ever lost.
2. Go's `time.Ticker` keeps the archive file rotation on time.
3. If there are errors when processing the stream, they are retried several
   times to recover if possible.

## Development

To contribute new features to this library, change into the source code directory:

    git clone <this repo>
    cd chirpradio-archiver

Here's how to run the tests:

    go test

If you want to add new dependencies, add them to the source code and re-run `go test`. Be sure to commit the changes to `go.mod` and `go.sum`.

The `chirpradio-archiver` binary is only compiled when you run `go build`
so, for development, run it directly to test local changes:

    go run archiver.go -help

## Bugs?

Probably! You can report them on the github issue tracker.

## Questions?

You can reach us at the [chirpdev Google Group](https://groups.google.com/forum/#!forum/chirpdev).
