"""Walks a `docker save` tarball and reconstructs the final root filesystem of the
image (layers applied in order, whiteouts honoured) without writing files to disk.

Each regular file is read once and hashed under every scheme in that single pass:
  file        SHA1 of the whole file (what tinycontainer's fileserver does)
  chunk_64k   SHA1 of each fixed-size 64 KiB chunk (the last chunk may be short)
  chunk_512k  same, 512 KiB
  chunk_4m    same, 4 MiB
"""

import hashlib
import json
import posixpath
import tarfile

SCHEMES = {"chunk_64k": 64 << 10, "chunk_512k": 512 << 10, "chunk_4m": 4 << 20}
BLOCK = 64 << 10  # the smallest chunk size; every other size is a multiple of it


class FileRec:
    __slots__ = ("size", "sha1", "chunks", "layer")

    def __init__(self, size, sha1, chunks, layer):
        self.size = size
        self.sha1 = sha1          # 20-byte digest
        self.chunks = chunks      # scheme -> list of (20-byte digest, length)
        self.layer = layer


class LinkRec:
    __slots__ = ("target", "layer")

    def __init__(self, target, layer):
        self.target = target
        self.layer = layer


def hash_stream(f):
    whole = hashlib.sha1()
    chunks = {s: [] for s in SCHEMES}
    open_chunks = {s: [hashlib.sha1(), 0] for s in SCHEMES}
    size = 0
    while True:
        block = f.read(BLOCK)
        # tar members can return short reads; top the block up so chunk boundaries line up
        while block and len(block) < BLOCK:
            more = f.read(BLOCK - len(block))
            if not more:
                break
            block += more
        if not block:
            break
        size += len(block)
        whole.update(block)
        for scheme, csize in SCHEMES.items():
            h = open_chunks[scheme]
            h[0].update(block)
            h[1] += len(block)
            if h[1] == csize:
                chunks[scheme].append((h[0].digest(), h[1]))
                open_chunks[scheme] = [hashlib.sha1(), 0]
    for scheme, h in open_chunks.items():
        if h[1]:
            chunks[scheme].append((h[0].digest(), h[1]))
    return size, whole.digest(), chunks


def _norm(name):
    while name.startswith("./"):
        name = name[2:]
    return posixpath.normpath("/" + name.lstrip("/"))


def _delete_subtree(entries, path, below_layer):
    prefix = path.rstrip("/") + "/"
    for p in [p for p in entries if (p == path or p.startswith(prefix)) and entries[p].layer < below_layer]:
        del entries[p]


def walk_image_tar(path, want_chunks=True):
    """Returns {abs_path: FileRec | LinkRec} for the final rootfs of the image."""
    entries = {}
    with tarfile.open(path) as outer:
        manifest = json.load(outer.extractfile("manifest.json"))
        layers = manifest[0]["Layers"]
        for li, layer_name in enumerate(layers):
            raw = outer.extractfile(layer_name)
            with tarfile.open(fileobj=raw, mode="r|*") as layer:
                for m in layer:
                    p = _norm(m.name)
                    base = posixpath.basename(p)
                    parent = posixpath.dirname(p)
                    if base == ".wh..wh..opq":
                        _delete_subtree_children(entries, parent, li)
                        continue
                    if base.startswith(".wh."):
                        _delete_subtree(entries, posixpath.join(parent, base[4:]), li)
                        continue
                    if m.isdir():
                        continue
                    # a new file/link replaces whatever was there, including a directory tree
                    if p in entries:
                        del entries[p]
                    if m.isreg():
                        f = layer.extractfile(m)
                        if want_chunks:
                            size, sha1, chunks = hash_stream(f)
                        else:
                            size, sha1, chunks = _hash_whole(f)
                        entries[p] = FileRec(size, sha1, chunks, li)
                    elif m.issym():
                        target = m.linkname
                        if not target.startswith("/"):
                            target = posixpath.normpath(posixpath.join(parent, target))
                        entries[p] = LinkRec(target, li)
                    elif m.islnk():
                        src = entries.get(_norm(m.linkname))
                        if isinstance(src, FileRec):
                            entries[p] = FileRec(src.size, src.sha1, src.chunks, li)
    return entries


def _delete_subtree_children(entries, directory, layer):
    prefix = directory.rstrip("/") + "/"
    for p in [p for p in entries if p.startswith(prefix) and entries[p].layer < layer]:
        del entries[p]


def _hash_whole(f):
    h = hashlib.sha1()
    size = 0
    while True:
        b = f.read(1 << 20)
        if not b:
            break
        size += len(b)
        h.update(b)
    return size, h.digest(), {}


def resolve(entries, path, max_hops=40):
    """Follows symlinks (including symlinked parent directories) to a regular file path,
    or returns None."""
    for _ in range(max_hops):
        rec = entries.get(path)
        if isinstance(rec, FileRec):
            return path
        if isinstance(rec, LinkRec):
            path = rec.target
            continue
        # maybe a parent directory is a symlink
        parts = path.strip("/").split("/")
        for i in range(len(parts) - 1, 0, -1):
            head = "/" + "/".join(parts[:i])
            prec = entries.get(head)
            if isinstance(prec, LinkRec):
                path = posixpath.normpath(posixpath.join(prec.target, *parts[i:]))
                break
        else:
            return None
    return None
