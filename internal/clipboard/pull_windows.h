#pragma once
#include <stdint.h>
#ifdef __cplusplus
extern "C" {
#endif
void *yd_pull_create(uintptr_t token, int count);
int yd_pull_entry(void *object, int index, const char *name, int64_t size, int directory);
long yd_pull_publish(void *object);
void yd_pull_discard(void *object);
#ifdef __cplusplus
}
#endif
