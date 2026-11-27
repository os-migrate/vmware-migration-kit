"""Guard against Ansible options that are parsed but never used.

The Golang modules decode the Ansible arguments into a ModuleArgs struct, then
copy the values into their own structures by hand. A field forgotten in that
copy keeps its Go zero value, so the option silently does nothing: the playbook
sets it, no error is raised, and the behaviour never changes.

That happened to the "assumezero" option of the migrate module. It was parsed,
documented, and the nbdcopy command builder honoured it and was unit tested,
but main() never copied it into MigrationConfig. Every migration therefore ran
without --destination-is-zero and wrote the zero ranges of the source disk for
real: about 70% of a typical system disk, and a 4x slowdown when measured.

A field counts as used when it is read through a variable holding the
ModuleArgs struct ("moduleArgs.Foo", "a.Foo"). Reading "c.Foo" from another
struct that happens to have the same field name does not count, which is
exactly how the assumezero bug hid.

The complementary case, a field read into a local variable that is then never
used, is already a compile error in Go ("declared and not used"), so the two
checks together cover the class.
"""

import pathlib
import re

import pytest

SRC = pathlib.Path(__file__).resolve().parent.parent / "plugins" / "modules" / "src"

# Options that are known to be decoded but unused. Each one is a bug: the
# playbook can set it and nothing happens. Listed here so this test guards
# every other option instead of failing on known issues.
KNOWN_UNUSED = {
    # create_server builds servers.CreateOpts without the keypair, so
    # "ssh_key_name" is silently dropped and instances get no SSH key.
    # "state" is accepted for symmetry with other modules and ignored.
    "create_server": {"KeyName", "State"},
}


def go_modules_with_args():
    return sorted(p for p in SRC.glob("*/*.go") if "type ModuleArgs struct" in p.read_text())


def module_args_fields(source):
    body = re.search(r"type ModuleArgs struct \{(.*?)\n\}", source, re.S)
    return re.findall(r"^\t(\w+)\s+\S", body.group(1), re.M) if body else []


def module_args_variables(source):
    """Names of the variables holding the decoded ModuleArgs."""
    names = set(re.findall(r"var\s+(\w+)\s+ModuleArgs\b", source))
    names |= set(re.findall(r"\((?:[^()]*?\b)?(\w+)\s+ModuleArgs\)", source))
    names |= set(re.findall(r"(\w+)\s*:=\s*ModuleArgs\{", source))
    return names


@pytest.mark.parametrize("path", go_modules_with_args(), ids=lambda p: p.parent.name)
def test_every_module_arg_is_used(path):
    source = path.read_text()
    module = path.parent.name
    fields = module_args_fields(source)
    assert fields, "no ModuleArgs fields found in %s" % path

    variables = module_args_variables(source)
    assert variables, "no variable of type ModuleArgs found in %s" % path

    unused = {
        field
        for field in fields
        if not any("%s.%s" % (var, field) in source for var in variables)
    }
    assert not unused - KNOWN_UNUSED.get(module, set()), (
        "%s: these Ansible options are decoded but never used, so setting them has no "
        "effect: %s" % (module, ", ".join(sorted(unused - KNOWN_UNUSED.get(module, set()))))
    )
