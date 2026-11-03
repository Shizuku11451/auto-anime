package com.autoanime.android

import android.os.Bundle
import android.os.StrictMode
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.hilt.lifecycle.viewmodel.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.autoanime.android.core.designsystem.AutoAnimeTheme
import com.autoanime.android.navigation.AutoAnimeRoot
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        if (BuildConfig.DEBUG) {
            StrictMode.setThreadPolicy(StrictMode.ThreadPolicy.Builder().detectAll().penaltyLog().build())
            StrictMode.setVmPolicy(StrictMode.VmPolicy.Builder().detectAll().penaltyLog().build())
        }
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            val app = hiltViewModel<AppViewModel>()
            val dynamicColor by app.dynamicColor.collectAsStateWithLifecycle()
            val session by app.session.collectAsStateWithLifecycle()
            AutoAnimeTheme(dynamicColor = dynamicColor) {
                AutoAnimeRoot(session)
            }
        }
    }
}
