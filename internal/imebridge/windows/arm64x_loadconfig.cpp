// Linker metadata for the code-free ARM64X forwarder. The fields and
// padding follow IMAGE_LOAD_CONFIG_DIRECTORY64 in the Windows SDK.
struct LoadConfig {
    unsigned size;
#ifdef _M_ARM64EC
    unsigned char before_chpe[0xc4];
    const void* chpe;
    unsigned char after_chpe[0x70];
#else
    unsigned char reserved[0x13c];
#endif
};

static_assert(sizeof(LoadConfig) == 0x140);

#ifdef _M_ARM64EC
static_assert(__builtin_offsetof(LoadConfig, chpe) == 0xc8);
// CHPE version 2. A pure forwarder has no code ranges, thunks or imports.
static const unsigned chpe_metadata[28] = {2};
extern "C" const LoadConfig _load_config_used = {sizeof(LoadConfig), {}, chpe_metadata, {}};
#else
extern "C" const LoadConfig _load_config_used = {sizeof(LoadConfig), {}};
#endif
