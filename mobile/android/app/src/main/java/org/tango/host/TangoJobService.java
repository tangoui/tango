package org.tango.host;

import android.app.job.JobParameters;
import android.app.job.JobService;

/**
 * Runs the background jobs sys/background schedules (Device.schedule). The
 * system may start the process just for a job: Go is started here then,
 * main.main and App.Run run without an Activity, and the job reaches Go as
 * an AppBackgroundTask event once the app has started.
 *
 * Declared in AndroidManifest.xml with the BIND_JOB_SERVICE permission, so
 * only the system can bind it.
 */
public class TangoJobService extends JobService {
    @Override
    public boolean onStartJob(JobParameters params) {
        Tango.start(this);
        Device.jobStarted(this, params);
        return true; // Go finishes it (Device.finishBackground → jobFinished)
    }

    @Override
    public boolean onStopJob(JobParameters params) {
        Device.jobStopped(params);
        return false; // one-shot: the app schedules the next run
    }
}
