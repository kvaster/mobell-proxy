#include <stddef.h>
#include <libavcodec/avcodec.h>

typedef struct
{
    unsigned char* data;
    size_t size;
    AVPacket pkt;
} Packet;

void* create();
void destroy(void* codec);
void onStreamStart(void* codec);
void onStreamStop(void* codec);
int onVideoPacket(void* codec, unsigned char* data, size_t size);
Packet* encodeFrame(void* codec);
void resetEncoder(void* codec, Packet* packet);
