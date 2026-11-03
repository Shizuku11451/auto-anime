package com.autoanime.android.feature.player

import androidx.media3.common.PlaybackException
import org.junit.Assert.assertEquals
import org.junit.Test

class PlaybackErrorTest {
    @Test fun non401HttpErrorsAreNotReportedAsExpiredSessions() {
        assertEquals(R.string.playback_http_failed, playbackErrorMessage(500, PlaybackException("http", null, 0)))
        assertEquals(R.string.playback_http_failed, playbackErrorMessage(403, PlaybackException("http", null, 0)))
    }

    @Test fun aMissingFileReportsAnUnavailableVideo() {
        assertEquals(R.string.video_unavailable, playbackErrorMessage(404, PlaybackException("http", null, 0)))
    }

    @Test fun decoderFailuresReportFormatProblems() {
        val error = PlaybackException("decode", null, PlaybackException.ERROR_CODE_DECODING_FORMAT_UNSUPPORTED)
        assertEquals(R.string.playback_format_failed, playbackErrorMessage(null, error))
    }
}
