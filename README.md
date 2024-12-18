next task:

get a docker image (or another far file) hashed and indexed on the fileserver

then try to implement the filesystem again. the reader should probably be from files not tar files this time
the file type can stay the same I think



to test: start the main on filesystem.go


additional test: point runc at the workers directory


TO RUN

user machine runs "sway" which takes their python program, tar balls it and sends it to the fileserver

worker machine runc on the special, mounted FUSE directory ("/app") (the runc config specifies which python directory to run)
Then all files that the script needs will be fetched from the fileserver

to setup

1. start the fileserver (admin)
2. run the script to populate fileserver (user)
3. on a linux machine, runc on sudo `runc run mycontainerid`



sample runc config:
{
  "ociVersion": "1.0.2-dev",
  "process": {
    "terminal": true,
    "user": {
      "uid": 0,
      "gid": 0
    },
    "args": [
      "python", "/app/my_script.py"
    ],
    "env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
      "TERM=xterm"
    ],
    "cwd": "/",
    "capabilities": {
      "bounding": [
        "CAP_AUDIT_WRITE",
        "CAP_KILL",
        "CAP_NET_BIND_SERVICE"
      ]
    }
  },
  "root": {
    "path": "/path/to/rootfs",
    "readonly": false
  },
  "mounts": [
    {
      "destination": "/app",
      "type": "fuse.myfuse",
      "source": "myfuse",
      "options": ["rw", "nosuid", "nodev", "relatime"]
    }
  ],
  "linux": {
    "namespaces": [
      {"type": "pid"},
      {"type": "network"}
    ]
  }
}