"""Keep an instrumented diagnostic worker distinct from a production packet."""
PRODUCTION = 'production'
EXHAUSTION = 'diagnostic-exhaustion-test-worker'
VARIANTS = (PRODUCTION, EXHAUSTION)


def variant(build):
    if type(build) is not dict:
        raise ValueError('invalid trial shim variant')
    # Frozen historical production packets predate explicit variant metadata.
    fields = {'shim_variant', 'instrumented_test_executable'} & set(build)
    if not fields:
        return PRODUCTION
    if fields != {'shim_variant', 'instrumented_test_executable'}:
        raise ValueError('incomplete trial shim variant')
    selected = build['shim_variant']
    instrumented = build['instrumented_test_executable']
    if (type(selected) is not str or selected not in VARIANTS or
            type(instrumented) is not bool or instrumented != (selected == EXHAUSTION)):
        raise ValueError('invalid trial shim variant')
    return selected


def require_production(build):
    if variant(build) != PRODUCTION:
        raise ValueError('instrumented diagnostic worker requires separate trial admission')


def require_admission(build,diagnostic_exhaustion=False):
    if type(diagnostic_exhaustion) is not bool:
        raise ValueError('invalid diagnostic exhaustion admission')
    if diagnostic_exhaustion:
        if variant(build)!=EXHAUSTION:
            raise ValueError('diagnostic exhaustion requires the instrumented packet')
    else:
        require_production(build)
