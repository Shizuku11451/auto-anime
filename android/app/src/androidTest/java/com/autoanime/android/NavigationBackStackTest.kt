package com.autoanime.android

import androidx.navigation.testing.TestNavHostController
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NavigationBackStackTest {
    @Test fun playerLogoutActionClearsProtectedDestinationsFromBackStack() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.runOnMainSync {
            val nav = TestNavHostController(instrumentation.targetContext)
            nav.setGraph(R.navigation.nav_graph)

            nav.navigate(R.id.action_connect_to_library)
            assertEquals(R.id.libraryFragment, nav.currentDestination?.id)
            nav.navigate(R.id.action_library_to_detail)
            assertEquals(R.id.detailFragment, nav.currentDestination?.id)
            nav.navigate(R.id.action_detail_to_player)
            assertEquals(R.id.playerFragment, nav.currentDestination?.id)

            nav.navigate(R.id.action_player_logout)
            assertEquals(R.id.connectFragment, nav.currentDestination?.id)
            assertNull("connect must be the only entry after logout", nav.previousBackStackEntry)
            assertFalse("root connect destination must not pop", nav.popBackStack())
        }
    }
}
