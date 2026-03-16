#define _GNU_SOURCE
#include <dlfcn.h>
#include <unistd.h>
#include <errno.h>
#include <signal.h>
#include <stdatomic.h>
#include <stdio.h>
#include <fcntl.h>
#include <sys/stat.h>

static int (*real_fdatasync)(int) = NULL;
static _Atomic int inject_enospc = 0;
static int marker_fd = -1;

static void toggle_handler(int signo){
int old_value = atomic_load(&inject_enospc);
int new_value = !old_value;
atomic_store(&inject_enospc, new_value);

// Signal-safe logging
if (marker_fd >= 0) {
        char msg[64];
        int len = snprintf(msg, sizeof(msg), "SIGUSR1 toggled %d->%d\n", old_value, new_value);
        write(marker_fd, msg, len);
}
}

__attribute__((constructor))
void init(){
// Create marker file to prove constructor ran
marker_fd = open("/tmp/fdatasync_marker", O_WRONLY|O_CREAT|O_TRUNC, 0644);
if (marker_fd >= 0) {
        write(marker_fd, "constructor_ran\n", 16);
        fsync(marker_fd);
}

// Find real fdatasync function
real_fdatasync = dlsym(RTLD_NEXT, "fdatasync");
if (!real_fdatasync && marker_fd >= 0) {
        write(marker_fd, "dlsym_failed\n", 14);
}

// Install signal handler
struct sigaction sa = {0};
sa.sa_handler = toggle_handler;
sigemptyset(&sa.sa_mask);
sa.sa_flags = SA_RESTART;

if (sigaction(SIGUSR1, &sa, NULL) == -1) {
        if (marker_fd >= 0) {
                write(marker_fd, "sigaction_failed\n", 18);
        }
} else {
        if (marker_fd >= 0) {
                write(marker_fd, "sigaction_success\n", 19);
        }
}
}

int fdatasync(int fd){
if(atomic_load(&inject_enospc)){
        errno = ENOSPC;
        return -1;
}
return real_fdatasync(fd);
}