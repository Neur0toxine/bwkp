#ifndef BWKP_KPDB_BRIDGE_H
#define BWKP_KPDB_BRIDGE_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    uint8_t* ptr;
    size_t len;
} bwkp_kpdb_buffer;

int32_t bwkp_kpdb_write(const char* path_ptr,
                         size_t path_len,
                         const uint8_t* database_ptr,
                         size_t database_len,
                         const uint8_t* credentials_ptr,
                         size_t credentials_len,
                         const uint8_t* options_ptr,
                         size_t options_len,
                         bwkp_kpdb_buffer* error);
int32_t bwkp_kpdb_verify(const char* path_ptr,
                          size_t path_len,
                          const uint8_t* credentials_ptr,
                          size_t credentials_len,
                          bwkp_kpdb_buffer* error);
int32_t bwkp_kpdb_read(const char* path_ptr,
                        size_t path_len,
                        const uint8_t* credentials_ptr,
                        size_t credentials_len,
                        bwkp_kpdb_buffer* output,
                        bwkp_kpdb_buffer* error);
const char* bwkp_keepassxc_version(void);
void bwkp_kpdb_buffer_free(bwkp_kpdb_buffer buffer);

#ifdef __cplusplus
}
#endif

#endif
