"""Select definitions from a separately hash-pinned, operator-reviewed fixture.

This is not an execution sandbox or a substitute for reviewing the prefix. A
caller must verify the complete source digest before compiling/executing it.
"""
import ast
import keyword


def before_assignment(source, name):
    """Return text before one ordinary top-level assignment, without replay.

    Strings, comments and nested assignments cannot choose the boundary. Refuse
    ambiguous/missing boundaries and compound same-line statements rather than
    silently omitting checks or selecting part of a statement.
    """
    if type(source) is not str or len(source) > 1 << 20:
        raise ValueError('invalid retained fixture source')
    if type(name) is not str or not name.isidentifier() or keyword.iskeyword(name):
        raise ValueError('invalid fixture boundary name')
    try:
        tree = ast.parse(source)
    except (SyntaxError, ValueError, RecursionError):
        raise ValueError('invalid retained fixture source') from None
    matches = [node for node in tree.body if isinstance(node, ast.Assign)
               and any(isinstance(target, ast.Name) and target.id == name
                       for target in node.targets)]
    if len(matches) != 1 or matches[0].col_offset != 0:
        raise ValueError('fixture boundary must be one top-level assignment')
    return ''.join(source.splitlines(keepends=True)[:matches[0].lineno - 1])
