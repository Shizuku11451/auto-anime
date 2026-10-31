package com.autoanime.android.feature

import androidx.media3.common.PlaybackException
import org.junit.Assert.assertEquals
import org.junit.Test

class PlaybackErrorTest {
    @Test fun non401HttpErrorsAreNotReportedAsExpiredSessions() {
        assertEquals(com.autoanime.android.R.string.playback_http_failed, playbackErrorMessage(404, PlaybackException("http", null, 0)))
        assertEquals(com.autoanime.android.R.string.playback_http_failed, playbackErrorMessage(500, PlaybackException("http", null, 0)))
    }
}
