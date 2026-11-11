"""Windows x64 executable header check shared by the release builder and the
Chocolatey package renderer, so both accept exactly the same binaries.

Header shape only: not a signature, version or dependency check.
"""
import struct


def x64_pe_problem(data):
    """Return None for an executable AMD64 PE32+ image, else the reason.

    `data` is the file's bytes (at least through the PE header).
    """
    if len(data) < 64 or data[:2] != b"MZ":
        return "not a Windows PE executable"
    offset = struct.unpack_from("<I", data, 60)[0]
    if offset < 64 or offset > len(data) - 26:
        return "invalid PE header offset"
    header = data[offset:offset + 26]
    machine = struct.unpack_from("<H", header, 4)[0]
    flags = struct.unpack_from("<H", header, 22)[0]
    magic = struct.unpack_from("<H", header, 24)[0]
    # x64 machine, PE32+ optional header, executable image, not a DLL.
    if header[:4] != b"PE\0\0" or machine != 0x8664 or magic != 0x20b or not flags & 2 or flags & 0x2000:
        return "expected Windows x64 PE32+ executable"
    return None
