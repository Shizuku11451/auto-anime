package com.autoanime.android

import android.os.Bundle
import android.os.StrictMode
import android.view.ViewTreeObserver
import androidx.appcompat.app.AppCompatActivity
import androidx.navigation.fragment.NavHostFragment
import com.autoanime.android.BuildConfig

class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (BuildConfig.DEBUG) {
            StrictMode.setThreadPolicy(
                StrictMode.ThreadPolicy.Builder()
                    .detectAll()
                    .penaltyLog()
                    .build()
            )
            StrictMode.setVmPolicy(
                StrictMode.VmPolicy.Builder()
                    .detectAll()
                    .penaltyLog()
                    .build()
            )
        }
        setContentView(R.layout.activity_main)
        val root = findViewById<android.view.View>(R.id.nav_host)
        root.viewTreeObserver.addOnPreDrawListener(object : ViewTreeObserver.OnPreDrawListener {
            override fun onPreDraw(): Boolean {
                root.viewTreeObserver.removeOnPreDrawListener(this)
                // Marks first-frame UI readiness, not connection, list, or cover completion.
                reportFullyDrawn()
                return true
            }
        })
        (supportFragmentManager.findFragmentById(R.id.nav_host) as NavHostFragment).navController
    }
}
