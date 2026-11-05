"""nbdkit Python plugin backed by OpenVixDiskLib.

Serves a VMware VMDK read-only over NBD using OpenVixDiskLib (a pure-Python
VDDK NBD-path replacement) instead of the proprietary VMware VDDK library.
The migration pipeline reads the source disk from this export and writes it to
the destination Cinder volume with nbdcopy, so only the read path is needed.

nbdkit invokes this as:

    nbdkit python openvixdisklib_nbdkit.py \
        server=<vcenter> user=<user> password=<pass> thumbprint=<sha1> \
        vm=moref=vm-123 [snapshot=snapshot-456] \
        disk="[datastore] vm/vm.vmdk" size=<bytes> \
        [transport=nbdssl|nbd] [port=443] [allow_untrusted=true]

The disk path may point at a snapshot delta (e.g. "...-000007.vmdk"); the
caller passes the snapshot-consistent backing file, which lets Change Block
Tracking (CBT) incremental copies work: the changed ranges are computed with
govmomi and only those sectors are read back through this export.
"""

import contextlib

import nbdkit

from openvixdisklib import nfc_auth
from openvixdisklib import openvixdisklib as vddk

API_VERSION = 2

SECTOR = vddk.VIXDISKLIB_SECTOR_SIZE  # 512

_params = {}


def config(key, value):
    _params[key] = value


def config_complete():
    for required in ("server", "user", "password", "vm", "disk", "size"):
        if not _params.get(required):
            raise RuntimeError("openvixdisklib: missing required parameter '%s'" % required)
    try:
        int(_params["size"])
    except (TypeError, ValueError):
        raise RuntimeError("openvixdisklib: 'size' must be an integer number of bytes")


def thread_model():
    # Each nbdkit connection gets its own independent OpenVixDiskLib NFC session
    # (see open()), so different connections can run concurrently. Within a single
    # connection the one op-id-sequenced NFC socket must not be used concurrently,
    # so serialize requests per connection. Combined with can_multi_conn=True this
    # lets nbdcopy --connections=N drive N parallel NFC sessions.
    return nbdkit.THREAD_MODEL_SERIALIZE_REQUESTS


def open(readonly):
    handle = vddk.VixDiskLibHandle(vixdisklib_compatibility_version="8.0")
    thumbprint = _params.get("thumbprint") or nfc_auth.get_ssl_cert_thumbprint(
        _params["server"]
    )
    snapshot = _params.get("snapshot") or None
    allow_untrusted = str(_params.get("allow_untrusted", "false")).lower() in (
        "1",
        "true",
        "yes",
    )

    stack = contextlib.ExitStack()
    try:
        conn = stack.enter_context(
            handle.connect(
                server_name=_params["server"],
                thumbprint=thumbprint,
                username=_params["user"],
                password=_params["password"],
                vmx_spec=_params["vm"],
                snapshot_ref=snapshot,
                transport_modes=_params.get("transport", "nbdssl"),
                port=int(_params.get("port", 443)),
                read_only=True,
                allow_untrusted=allow_untrusted,
            )
        )
        disk = stack.enter_context(
            handle.open(
                conn,
                _params["disk"],
                flags=vddk.VIXDISKLIB_FLAG_OPEN_READ_ONLY,
            )
        )
    except Exception:
        stack.close()
        raise

    return {
        "handle": handle,
        "disk": disk,
        "stack": stack,
        "size": int(_params["size"]),
    }


def get_size(h):
    return h["size"]


def can_write(h):
    return False


def can_multi_conn(h):
    # Each connection opens its own NFC session, so multiple connections are safe
    # and independent. This lets nbdcopy use --connections=N for parallelism.
    return True


def pread(h, buf, offset, flags):
    length = len(buf)
    # OpenVixDiskLib reads whole 512-byte sectors; nbdcopy and the CBT path use
    # sector-aligned requests, but read the surrounding sectors and slice to be
    # safe on any unaligned edge.
    first_sector = offset // SECTOR
    last_sector = (offset + length - 1) // SECTOR
    num_sectors = last_sector - first_sector + 1
    tmp = vddk.get_buffer(num_sectors * SECTOR)
    h["handle"].read(h["disk"], first_sector, num_sectors, tmp)
    start = offset - first_sector * SECTOR
    buf[:] = bytes(tmp)[start:start + length]


def close(h):
    h["stack"].close()
