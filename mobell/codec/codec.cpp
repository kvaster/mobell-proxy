// c part of decoder/encoder

extern "C"
{
    #include "codec.h"
    #include <libavcodec/avcodec.h>
    #include <libavutil/rational.h>
}

#include <pthread.h>

class Codec {
public:
    Codec();
    ~Codec();

    void OnStreamStart();
    void OnStreamStop();
    bool OnVideoPacket(unsigned char* data, size_t size);

    Packet* EncodeFrame();
    void ResetEncoder(Packet* packet);

private:
    AVCodec* videoCodec;
    AVCodecContext* videoCodecCtx;
    AVFrame* videoFrame;
    AVFrame* videoWorkFrame;

    AVCodec* jpegCodec;

    AVPacket* pkt;

    pthread_mutex_t videoMutex;
};

extern "C" void* create()
{
    return (void*)new Codec();
}

extern "C" void destroy(void* codec)
{
    delete (Codec*)codec;
}

extern "C" void onStreamStart(void* codec)
{
    ((Codec*)codec)->OnStreamStart();
}

extern "C" void onStreamStop(void* codec)
{
    ((Codec*)codec)->OnStreamStop();
}

extern "C" int onVideoPacket(void* codec, unsigned char* data, size_t size)
{
    return ((Codec*)codec)->OnVideoPacket(data, size) ? 0 : -1;
}

extern "C" Packet* encodeFrame(void* codec)
{
    return ((Codec*)codec)->EncodeFrame();
}

extern "C" void resetEncoder(void* codec, Packet* packet)
{
    ((Codec*)codec)->ResetEncoder(packet);
}


Codec::Codec()
{
    pthread_mutex_init(&videoMutex, nullptr);

    videoCodec = avcodec_find_decoder(AV_CODEC_ID_MXPEG);
    videoCodecCtx = nullptr;
    videoFrame = av_frame_alloc();
    videoWorkFrame = av_frame_alloc();

    jpegCodec = avcodec_find_encoder(AV_CODEC_ID_MJPEG);

    pkt = av_packet_alloc();
}

Codec::~Codec()
{
    av_packet_free(&pkt);

    avcodec_free_context(&videoCodecCtx);
    av_frame_free(&videoFrame);
    av_frame_free(&videoWorkFrame);

    pthread_mutex_destroy(&videoMutex);
}

void Codec::OnStreamStart()
{
    videoCodecCtx = avcodec_alloc_context3(videoCodec);
    avcodec_open2(videoCodecCtx, videoCodec, nullptr);
}

void Codec::OnStreamStop()
{
    av_frame_unref(videoFrame);

    if (videoCodecCtx)
        avcodec_free_context(&videoCodecCtx);
}

bool Codec::OnVideoPacket(unsigned char* data, size_t size)
{
    pkt->data = data;
    pkt->size = size;

    avcodec_send_packet(videoCodecCtx, pkt);

    pthread_mutex_lock(&videoMutex);

    bool ok = true;
    int ret = 0;
    while (ret >= 0)
    {
        ret = avcodec_receive_frame(videoCodecCtx, videoWorkFrame);

        if (ret == AVERROR(EAGAIN) || ret == AVERROR_EOF)
            break;

        if (ret < 0) // error
        {
            ok = false;
            break;
        }

        av_frame_unref(videoFrame);
        av_frame_ref(videoFrame, videoWorkFrame);
    }

    pthread_mutex_unlock(&videoMutex);

    return ok;
}

Packet* Codec::EncodeFrame()
{
    AVCodecContext* jpegCodecCtx = avcodec_alloc_context3(jpegCodec);

    Packet* p = new Packet();
    p->data = nullptr;
    p->size = 0;
    p->pkt = av_packet_alloc();

    pthread_mutex_lock(&videoMutex);

    if ((videoFrame->width > 0) && (videoFrame->height > 0)) {
        jpegCodecCtx->pix_fmt = videoCodecCtx->pix_fmt;
        jpegCodecCtx->width = videoFrame->width;
        jpegCodecCtx->height = videoFrame->height;
        jpegCodecCtx->time_base = (AVRational){1,2};

        avcodec_open2(jpegCodecCtx, jpegCodec, nullptr);

        int gotFrame;
        avcodec_send_frame(jpegCodecCtx, videoFrame);
    }

    pthread_mutex_unlock(&videoMutex);

    avcodec_receive_packet(jpegCodecCtx, p->pkt);

    avcodec_close(jpegCodecCtx);

    p->data = p->pkt->data;
    p->size = p->pkt->size;

    return p;
}

void Codec::ResetEncoder(Packet* p)
{
    av_packet_free(&p->pkt);
    delete p;
}
