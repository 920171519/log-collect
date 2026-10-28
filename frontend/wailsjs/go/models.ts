export namespace model {
	
	export class DeviceConfig {
	    ip: string;
	    port: number;
	    username: string;
	    password: string;
	    commands: string;
	    collectPaths: string[];
	    endMarker: string;
	
	    static createFrom(source: any = {}) {
	        return new DeviceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.commands = source["commands"];
	        this.collectPaths = source["collectPaths"];
	        this.endMarker = source["endMarker"];
	    }
	}
	export class HostConfig {
	    ip: string;
	    port: number;
	    username: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new HostConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class Combination {
	    id: string;
	    name: string;
	    cloud: HostConfig;
	    deviceA: DeviceConfig;
	    deviceB: DeviceConfig;
	
	    static createFrom(source: any = {}) {
	        return new Combination(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cloud = this.convertValues(source["cloud"], HostConfig);
	        this.deviceA = this.convertValues(source["deviceA"], DeviceConfig);
	        this.deviceB = this.convertValues(source["deviceB"], DeviceConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppConfig {
	    version: number;
	    downloadDir: string;
	    archiveNameTemplate: string;
	    cloudStageDir: string;
	    deviceTempDir: string;
	    keepCloudCopy: boolean;
	    timeoutMinutes: number;
	    combinations: Combination[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.downloadDir = source["downloadDir"];
	        this.archiveNameTemplate = source["archiveNameTemplate"];
	        this.cloudStageDir = source["cloudStageDir"];
	        this.deviceTempDir = source["deviceTempDir"];
	        this.keepCloudCopy = source["keepCloudCopy"];
	        this.timeoutMinutes = source["timeoutMinutes"];
	        this.combinations = this.convertValues(source["combinations"], Combination);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DeviceRunState {
	    role: string;
	    ip: string;
	    phase: string;
	    message: string;
	    archiveName: string;
	    cloudPath: string;
	    localPath: string;
	    bytes: number;
	    startedAt: string;
	    finishedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new DeviceRunState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.ip = source["ip"];
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.archiveName = source["archiveName"];
	        this.cloudPath = source["cloudPath"];
	        this.localPath = source["localPath"];
	        this.bytes = source["bytes"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}
	export class RunState {
	    comboId: string;
	    comboName: string;
	    phase: string;
	    message: string;
	    devices: DeviceRunState[];
	    localFiles: string[];
	    startedAt: string;
	    finishedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new RunState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.comboId = source["comboId"];
	        this.comboName = source["comboName"];
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.devices = this.convertValues(source["devices"], DeviceRunState);
	        this.localFiles = source["localFiles"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppState {
	    config: AppConfig;
	    configPath: string;
	    configError: string;
	    runs: RunState[];
	
	    static createFrom(source: any = {}) {
	        return new AppState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], AppConfig);
	        this.configPath = source["configPath"];
	        this.configError = source["configError"];
	        this.runs = this.convertValues(source["runs"], RunState);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class LogEntry {
	    seq: number;
	    comboId: string;
	    device: string;
	    level: string;
	    message: string;
	    at: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.comboId = source["comboId"];
	        this.device = source["device"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.at = source["at"];
	    }
	}

}

